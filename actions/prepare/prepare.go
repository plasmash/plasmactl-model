package prepare

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/launchrctl/launchr/pkg/action"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

//go:embed library
var libraryFS embed.FS

// Known component types that indicate a directory is a layer
var componentTypes = map[string]bool{
	"applications": true,
	"services":     true,
	"softwares":    true,
	"entities":     true,
	"metrics":      true,
	"flows":        true,
	"skills":       true,
	"functions":    true,
	"executors":    true,
	"helpers":      true,
	"libraries":    true,
	"variables":    true,
	"group_vars":   true,
	"actions":      true,
}

// Prepare implements the model:prepare command
type Prepare struct {
	action.WithLogger
	action.WithTerm

	ComposeDir string
	PrepareDir string
	Clean      bool

	layers []string
}

// Execute runs the model:prepare action
func (p *Prepare) Execute() error {
	// Clean prepare directory if requested
	if p.Clean {
		p.Term().Info().Printfln("Cleaning prepare directory: %s", p.PrepareDir)
		if err := os.RemoveAll(p.PrepareDir); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to clean prepare directory: %w", err)
		}
	}

	// Create prepare directory
	if err := os.MkdirAll(p.PrepareDir, 0755); err != nil {
		return fmt.Errorf("failed to create prepare directory: %w", err)
	}

	// Check if compose directory exists
	if _, err := os.Stat(p.ComposeDir); os.IsNotExist(err) {
		return fmt.Errorf("compose directory not found: %s (run model:compose first)", p.ComposeDir)
	}

	p.Term().Info().Printfln("Copying from %s", p.ComposeDir)
	if err := p.copyComposeImage(); err != nil {
		return fmt.Errorf("failed to copy compose image: %w", err)
	}

	p.Term().Info().Println("Preparing Ansible runtime...")

	// Structure transformations
	if err := p.flattenSrcDirectory(); err != nil {
		return err
	}

	p.layers = p.discoverLayers()

	componentsMoved, err := p.createRolesStructure()
	if err != nil {
		return err
	}
	p.Term().Info().Printfln("  ✓ Moved %d components to roles/", componentsMoved)

	layersRenamed, err := p.renameVariablesToGroupVars()
	if err != nil {
		return err
	}
	p.Term().Info().Printfln("  ✓ Renamed variables/ to group_vars/ in %d layers", layersRenamed)

	galaxyCount, err := p.generateGalaxyFiles()
	if err != nil {
		return err
	}
	p.Term().Info().Printfln("  ✓ Generated %d galaxy.yml files", galaxyCount)

	symlinksCreated, err := p.createPlatformSymlinks()
	if err != nil {
		return err
	}
	p.Term().Info().Printfln("  ✓ Created %d platform symlinks", symlinksCreated)

	if err := p.createAnsibleCfg(); err != nil {
		return err
	}
	p.Term().Info().Println("  ✓ Created ansible.cfg")

	if err := p.createAnsibleCollectionsSymlink(); err != nil {
		return err
	}

	// Copy library if it exists in compose output
	if err := p.copyLibrary(); err != nil {
		p.Term().Warning().Printfln("  ! Library not copied: %v", err)
	} else {
		p.Term().Info().Println("  ✓ Copied library/")
	}

	p.Term().Success().Println("Preparation completed.")
	return nil
}

// copyComposeImage copies compose image to prepare directory, excluding hidden directories
func (p *Prepare) copyComposeImage() error {
	return filepath.Walk(p.ComposeDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") && path != p.ComposeDir {
			return filepath.SkipDir
		}

		// Get relative path
		relPath, err := filepath.Rel(p.ComposeDir, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(p.PrepareDir, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, destPath)
		}

		// Copy regular file
		return copyFile(path, destPath)
	})
}

// flattenSrcDirectory flattens src/ directory to root if present
func (p *Prepare) flattenSrcDirectory() error {
	srcDir := filepath.Join(p.PrepareDir, "src")
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		destPath := filepath.Join(p.PrepareDir, entry.Name())

		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			if err := os.Rename(srcPath, destPath); err != nil {
				return err
			}
		}
	}

	// Remove empty src directory
	os.Remove(srcDir)
	p.Term().Info().Println("  ✓ Flattened src/")
	return nil
}

// discoverLayers discovers layers by looking at directories with known component types
func (p *Prepare) discoverLayers() []string {
	var layers []string

	entries, err := os.ReadDir(p.PrepareDir)
	if err != nil {
		return layers
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		// A layer has subdirectories with known component type names
		layerPath := filepath.Join(p.PrepareDir, entry.Name())
		subdirs, err := os.ReadDir(layerPath)
		if err != nil {
			continue
		}

		for _, subdir := range subdirs {
			if subdir.IsDir() && componentTypes[subdir.Name()] {
				layers = append(layers, entry.Name())
				break
			}
		}
	}

	sort.Strings(layers)
	return layers
}

// createRolesStructure creates roles/ structure for Ansible
func (p *Prepare) createRolesStructure() (int, error) {
	componentsMoved := 0

	for _, layer := range p.layers {
		layerDir := filepath.Join(p.PrepareDir, layer)

		typeDirs, err := os.ReadDir(layerDir)
		if err != nil {
			continue
		}

		for _, typeDir := range typeDirs {
			if !typeDir.IsDir() {
				continue
			}

			// Skip non-component directories
			typeName := typeDir.Name()
			if typeName == "variables" || typeName == "actions" || typeName == "docs" {
				continue
			}

			typePath := filepath.Join(layerDir, typeName)
			rolesDir := filepath.Join(typePath, "roles")

			components, err := os.ReadDir(typePath)
			if err != nil {
				continue
			}

			var componentsToMove []string
			for _, comp := range components {
				if !comp.IsDir() {
					continue
				}
				// Skip roles/ and non-component directories
				if comp.Name() == "roles" || comp.Name() == "actions" || comp.Name() == "docs" {
					continue
				}
				componentsToMove = append(componentsToMove, comp.Name())
			}

			if len(componentsToMove) > 0 {
				if err := os.MkdirAll(rolesDir, 0755); err != nil {
					return componentsMoved, err
				}

				for _, compName := range componentsToMove {
					srcPath := filepath.Join(typePath, compName)
					destPath := filepath.Join(rolesDir, compName)
					if err := os.Rename(srcPath, destPath); err != nil {
						return componentsMoved, err
					}
					componentsMoved++
				}
			}
		}
	}

	return componentsMoved, nil
}

// renameVariablesToGroupVars renames variables/ to group_vars/ for Ansible compatibility
func (p *Prepare) renameVariablesToGroupVars() (int, error) {
	count := 0

	for _, layer := range p.layers {
		variablesDir := filepath.Join(p.PrepareDir, layer, "variables")
		groupVarsDir := filepath.Join(p.PrepareDir, layer, "group_vars")

		if _, err := os.Stat(variablesDir); os.IsNotExist(err) {
			continue
		}

		if err := os.Rename(variablesDir, groupVarsDir); err != nil {
			return count, err
		}
		count++

		// Flatten any nested variables/ directory inside group_vars/
		nestedVars := filepath.Join(groupVarsDir, "variables")
		if _, err := os.Stat(nestedVars); err == nil {
			entries, err := os.ReadDir(nestedVars)
			if err != nil {
				continue
			}

			for _, entry := range entries {
				srcPath := filepath.Join(nestedVars, entry.Name())
				destPath := filepath.Join(groupVarsDir, entry.Name())
				if _, err := os.Stat(destPath); os.IsNotExist(err) {
					os.Rename(srcPath, destPath)
				}
			}
			os.RemoveAll(nestedVars)
		}
	}

	return count, nil
}

// ansibleCfgData holds template data for ansible.cfg
type ansibleCfgData struct {
	CollectionsPath string
}

// createAnsibleCfg creates ansible.cfg using the embedded template
func (p *Prepare) createAnsibleCfg() error {
	ansibleCfg := filepath.Join(p.PrepareDir, "ansible.cfg")

	if _, err := os.Stat(ansibleCfg); err == nil {
		return nil // Already exists
	}

	tmplContent, err := templatesFS.ReadFile("templates/ansible.cfg.tmpl")
	if err != nil {
		return fmt.Errorf("failed to read ansible.cfg template: %w", err)
	}

	tmpl, err := template.New("ansible.cfg").Parse(string(tmplContent))
	if err != nil {
		return fmt.Errorf("failed to parse ansible.cfg template: %w", err)
	}

	var buf bytes.Buffer
	data := ansibleCfgData{
		CollectionsPath: ".",
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute ansible.cfg template: %w", err)
	}

	return os.WriteFile(ansibleCfg, buf.Bytes(), 0644)
}

// createAnsibleCollectionsSymlink creates ansible_collections symlink
func (p *Prepare) createAnsibleCollectionsSymlink() error {
	symlink := filepath.Join(p.PrepareDir, "ansible_collections")

	if _, err := os.Lstat(symlink); err == nil {
		return nil // Already exists
	}

	return os.Symlink(".", symlink)
}

// copyLibrary extracts embedded library/ to prepare directory
func (p *Prepare) copyLibrary() error {
	libraryDest := filepath.Join(p.PrepareDir, "library")

	if _, err := os.Stat(libraryDest); err == nil {
		return nil // Already exists
	}

	// Extract embedded library/ to prepare directory
	return fs.WalkDir(libraryFS, "library", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Get relative path from "library" root
		relPath, err := filepath.Rel("library", path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(libraryDest, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		// Read file from embedded FS
		content, err := libraryFS.ReadFile(path)
		if err != nil {
			return err
		}

		// Write to destination
		return os.WriteFile(destPath, content, 0644)
	})
}

// createPlatformSymlinks creates platform symlinks in layer group_vars directories
func (p *Prepare) createPlatformSymlinks() (int, error) {
	count := 0

	for _, layer := range p.layers {
		if layer == "platform" {
			continue
		}

		groupVarsDir := filepath.Join(p.PrepareDir, layer, "group_vars")
		if _, err := os.Stat(groupVarsDir); os.IsNotExist(err) {
			continue
		}

		platformLink := filepath.Join(groupVarsDir, "platform")
		if _, err := os.Lstat(platformLink); err == nil {
			continue // Already exists
		}

		if err := os.Symlink("../../platform/group_vars/platform", platformLink); err != nil {
			return count, err
		}
		count++
	}

	return count, nil
}

// getVersion gets version from git tag, fallback to 1.0.0
func (p *Prepare) getVersion() string {
	r, err := git.PlainOpen(".")
	if err != nil {
		return "1.0.0"
	}

	head, err := r.Head()
	if err != nil {
		return "1.0.0"
	}

	tags, err := r.Tags()
	if err != nil {
		return "1.0.0"
	}

	var latestTag string
	_ = tags.ForEach(func(ref *plumbing.Reference) error {
		if ref.Hash() == head.Hash() {
			latestTag = ref.Name().Short()
		}
		return nil
	})

	if latestTag != "" {
		return strings.TrimPrefix(latestTag, "v")
	}

	return head.Hash().String()[:7]
}

// galaxyYmlData holds template data for galaxy.yml
type galaxyYmlData struct {
	Namespace string
	Name      string
	Version   string
}

// generateGalaxyFiles generates galaxy.yml files for Ansible Galaxy collections
func (p *Prepare) generateGalaxyFiles() (int, error) {
	version := p.getVersion()
	count := 0

	tmplContent, err := templatesFS.ReadFile("templates/galaxy.yml.tmpl")
	if err != nil {
		return 0, fmt.Errorf("failed to read galaxy.yml template: %w", err)
	}

	tmpl, err := template.New("galaxy.yml").Parse(string(tmplContent))
	if err != nil {
		return 0, fmt.Errorf("failed to parse galaxy.yml template: %w", err)
	}

	for _, layer := range p.layers {
		layerDir := filepath.Join(p.PrepareDir, layer)

		typeDirs, err := os.ReadDir(layerDir)
		if err != nil {
			continue
		}

		for _, typeDir := range typeDirs {
			if !typeDir.IsDir() {
				continue
			}

			// Skip non-component directories
			typeName := typeDir.Name()
			if typeName == "group_vars" || typeName == "actions" || typeName == "docs" {
				continue
			}

			galaxyFile := filepath.Join(layerDir, typeName, "galaxy.yml")
			if _, err := os.Stat(galaxyFile); err == nil {
				continue // Already exists
			}

			var buf bytes.Buffer
			data := galaxyYmlData{
				Namespace: layer,
				Name:      typeName,
				Version:   version,
			}

			if err := tmpl.Execute(&buf, data); err != nil {
				return count, fmt.Errorf("failed to execute galaxy.yml template: %w", err)
			}

			if err := os.WriteFile(galaxyFile, buf.Bytes(), 0644); err != nil {
				return count, err
			}
			count++
		}
	}

	return count, nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
