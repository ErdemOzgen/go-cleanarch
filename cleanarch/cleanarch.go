package cleanarch

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Layer represents software layers.
type Layer string

// Log is logger used by go-cleanarch.
//
// Can be replaced with any logger compatible logger.
// By default is io.writer is ioutil.Discard.
var Log = log.New(ioutil.Discard, "[cleanarch] ", log.LstdFlags|log.Lshortfile)

const (
	// LayerDomain is domain layer.
	LayerDomain Layer = "domain"

	// LayerApplication is application layer.
	LayerApplication Layer = "application"

	// LayerInfrastructure is infrastructure layer.
	LayerInfrastructure Layer = "infrastructure"

	// LayerInterfaces is interfaces layer.
	LayerInterfaces Layer = "interfaces"
)

var layersHierarchy = map[Layer]int{
	LayerDomain:         1,
	LayerApplication:    2,
	LayerInterfaces:     3,
	LayerInfrastructure: 4,
}

// NewValidator creates new Validator.
func NewValidator(alias map[string]Layer) *Validator {
	filesMetadata := make(map[string]LayerMetadata, 0)
	return &Validator{
		filesMetadata: filesMetadata,
		alias:         alias,
	}
}

// ValidationError represents error when Clean Architecture rule is not keep.
type ValidationError error

// Validator is responsible for Clean Architecture validation.
type Validator struct {
	filesMetadata map[string]LayerMetadata
	alias         map[string]Layer
}

// Validate validates provided path for Clean Architecture rules.
func (v *Validator) Validate(root string, ignoreTests bool, ignoredPackages []string) (bool, []ValidationError, error) {
	errors := []ValidationError{}

	err := filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if fi.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		if ignoreTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}

		if strings.Contains(path, "/vendor/") {
			// todo - better check and flag
			return nil
		}

		if strings.Contains(path, "/.") {
			return nil
		}

		fset := token.NewFileSet()

		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			panic(err)
		}

		Log.Print("processing: ", path)
		importerMeta := v.fileMetadata(path)
		Log.Printf("metadata: %+v", importerMeta)

		if importerMeta.Layer == "" || importerMeta.Module == "" {
			// todo - error from meta parser?
			Log.Printf("cannot parse metadata for file %s, meta: %+v", path, importerMeta)
			return nil
		}

	ImportsLoop:
		for _, imp := range f.Imports {
			for _, ignoredPackage := range ignoredPackages {
				if strings.Contains(imp.Path.Value, ignoredPackage) {
					continue ImportsLoop
				}
			}

			validationErrors := v.validateImport(imp, importerMeta, path)
			errors = append(errors, validationErrors...)
		}

		return nil
	})
	if err != nil {
		return false, nil, err
	}

	return len(errors) == 0, errors, nil
}

func (v *Validator) validateImport(imp *ast.ImportSpec, importerMeta LayerMetadata, path string) []ValidationError {
	errors := []ValidationError{}

	importPath := imp.Path.Value
	importPath = strings.TrimSuffix(importPath, `"`)
	importPath = strings.TrimPrefix(importPath, `"`)
	importMeta := v.fileMetadata(importPath)

	Log.Printf("import: %s, importMeta: %+v", importPath, importMeta)

	if importMeta.Module == importerMeta.Module {
		importHierarchy := layersHierarchy[importMeta.Layer]
		importerHierarchy := layersHierarchy[importerMeta.Layer]
		Log.Printf("import hierarchy: %d, importer hierarchy: %d", importHierarchy, importerHierarchy)

		if importHierarchy > importerHierarchy {
			err := fmt.Errorf(
				"you cannot import %s Layer (%s) to %s Layer (%s)",
				importMeta.Layer, importPath,
				importerMeta.Layer, path,
			)
			errors = append(errors, err)
		}
	} else if importMeta.Layer != "" {
		if importMeta.Layer != LayerInterfaces || importerMeta.Layer != LayerInfrastructure {
			err := fmt.Errorf(
				"trying to import %s Layer (%s) to %s Layer (%s) between %s and %s modules, you can only import interfaces Layer to infrastructure Layer",
				importMeta.Layer, importPath,
				importerMeta.Layer, path,
				importMeta.Module, importerMeta.Module,
			)
			errors = append(errors, err)
		}
	}
	return errors
}

func (v *Validator) fileMetadata(path string) LayerMetadata {
	if metadata, ok := v.filesMetadata[path]; ok {
		return metadata
	}

	v.filesMetadata[path] = ParseLayerMetadata(path, v.alias)
	return v.filesMetadata[path]
}

// LayerMetadata contains informations about directory module and software layer.
type LayerMetadata struct {
	Module string
	Layer  Layer
}

// ParseLayerMetadata parses metadata of provided path.
func ParseLayerMetadata(path string, alias map[string]Layer) LayerMetadata {
	pathParts := strings.Split(path, "/")

	metadata := LayerMetadata{}

	for i := len(pathParts) - 1; i >= 0; i-- {
		pathPart := pathParts[i]

		// we assume that the path upper the Layer is module name
		if metadata.Layer != "" {
			metadata.Module = pathPart
			break
		}

		for alias, layer := range alias {
			if pathPart == alias {
				metadata.Layer = layer
				continue
			}
		}
	}

	return metadata
}
