package testkit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GenerateGoProject writes a synthetic Go project to dir with the given tier's
// complexity. Creates go.mod, packages with functions, and cross-package calls.
func GenerateGoProject(dir string, tier ScaleTier) error {
	// go.mod
	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/stress\ngo 1.21\n"), 0o644); err != nil {
		return err
	}

	components := GenerateComponentNames(tier)

	// Generate source files per component
	for i, comp := range components {
		pkgDir := filepath.Join(dir, comp)
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			return err
		}
		pkgName := fmt.Sprintf("comp_%d", i)

		var src strings.Builder
		src.WriteString(fmt.Sprintf("package %s\n\n", pkgName))

		// Import a few other packages for cross-package calls
		var imports []int
		for j := 0; j < 3 && i+j+1 < len(components); j++ {
			imports = append(imports, i+j+1)
		}
		if len(imports) > 0 {
			src.WriteString("import (\n")
			for _, idx := range imports {
				src.WriteString(fmt.Sprintf("\t%s \"example.com/stress/%s\"\n",
					fmt.Sprintf("comp_%d", idx), components[idx]))
			}
			src.WriteString(")\n\n")
		}

		// Struct
		src.WriteString(fmt.Sprintf("type Config_%d struct {\n\tName string\n}\n\n", i))

		// Functions with typed signatures
		for f := 0; f < symbolsPerComponent; f++ {
			src.WriteString(fmt.Sprintf(
				"func Func_%d_%d(path string) *Config_%d {\n", i, f, i))
			// Call into imported packages
			for _, idx := range imports {
				src.WriteString(fmt.Sprintf(
					"\t_ = comp_%d.Func_%d_0(\"x\")\n", idx, idx))
			}
			src.WriteString(fmt.Sprintf(
				"\treturn &Config_%d{Name: path}\n}\n\n", i))
		}

		if err := os.WriteFile(filepath.Join(pkgDir, pkgName+".go"),
			[]byte(src.String()), 0o644); err != nil {
			return err
		}
	}
	return nil
}
