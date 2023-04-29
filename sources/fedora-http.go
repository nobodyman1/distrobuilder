package sources

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/lxc/distrobuilder/shared"
)

type fedora struct {
	common
}

// Run downloads a container base image and unpacks it and its layers.
func (s *fedora) Run() error {
	baseURL := fmt.Sprintf("%s/packages/Fedora-Container-Base",
		s.definition.Source.URL)

	// Get latest build
	build, err := s.getLatestBuild(baseURL, s.definition.Image.Release)
	if err != nil {
		return fmt.Errorf("Failed to get latest build: %w", err)
	}

	fname := fmt.Sprintf("Fedora-Container-Base-%s-%s.%s.tar.xz",
		s.definition.Image.Release, build, s.definition.Image.ArchitectureMapped)

	// Download image
	sourceURL := fmt.Sprintf("%s/%s/%s/images/%s", baseURL, s.definition.Image.Release, build, fname)

	fpath, err := s.DownloadHash(s.definition.Image, sourceURL, "", nil)
	if err != nil {
		return fmt.Errorf("Failed to download %q: %w", sourceURL, err)
	}

	s.logger.WithField("file", filepath.Join(fpath, fname)).Info("Unpacking image")

	// Unpack the base image
	err = shared.Unpack(filepath.Join(fpath, fname), s.rootfsDir)
	if err != nil {
		return fmt.Errorf("Failed to unpack %q: %w", filepath.Join(fpath, fname), err)
	}

	s.logger.Info("Unpacking layers")

	// Unpack the rest of the image (/bin, /sbin, /usr, etc.)
	err = s.unpackLayers(s.rootfsDir)
	if err != nil {
		return fmt.Errorf("Failed to unpack: %w", err)
	}

	return nil
}

func (s *fedora) unpackLayers(rootfsDir string) error {
	// Read manifest file which contains the path to the layers
	file, err := os.Open(filepath.Join(rootfsDir, "manifest.json"))
	if err != nil {
		return fmt.Errorf("Failed to open %q: %w", filepath.Join(rootfsDir, "manifest.json"), err)
	}

	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("Failed to read file %q: %w", file.Name(), err)
	}

	// Structure of the manifest excluding RepoTags
	var manifests []struct {
		Layers []string
		Config string
	}

	err = json.Unmarshal(data, &manifests)
	if err != nil {
		return fmt.Errorf("Failed to unmarshal JSON data: %w", err)
	}

	pathsToRemove := []string{
		filepath.Join(rootfsDir, "manifest.json"),
		filepath.Join(rootfsDir, "repositories"),
	}

	// Unpack tarballs (or layers) which contain the rest of the rootfs, and
	// remove files not relevant to the image.
	for _, manifest := range manifests {
		for _, layer := range manifest.Layers {
			s.logger.WithField("file", filepath.Join(rootfsDir, layer)).Info("Unpacking layer")

			err := shared.Unpack(filepath.Join(rootfsDir, layer), rootfsDir)
			if err != nil {
				return fmt.Errorf("Failed to unpack %q: %w", filepath.Join(rootfsDir, layer), err)
			}

			pathsToRemove = append(pathsToRemove,
				filepath.Join(rootfsDir, filepath.Dir(layer)))
		}

		pathsToRemove = append(pathsToRemove, filepath.Join(rootfsDir, manifest.Config))
	}

	// Clean up /tmp since there are unnecessary files there
	files, err := filepath.Glob(filepath.Join(rootfsDir, "tmp", "*"))
	if err != nil {
		return fmt.Errorf("Failed to find matching files: %w", err)
	}

	pathsToRemove = append(pathsToRemove, files...)

	// Clean up /root since there are unnecessary files there
	files, err = filepath.Glob(filepath.Join(rootfsDir, "root", "*"))
	if err != nil {
		return fmt.Errorf("Failed to find matching files: %w", err)
	}

	pathsToRemove = append(pathsToRemove, files...)

	for _, f := range pathsToRemove {
		os.RemoveAll(f)
	}

	return nil
}

func (s *fedora) getLatestBuild(URL, release string) (string, error) {
	var (
		resp *http.Response
		err  error
	)

	err = shared.Retry(func() error {
		resp, err = http.Get(fmt.Sprintf("%s/%s", URL, release))
		if err != nil {
			return fmt.Errorf("Failed to GET %q: %w", fmt.Sprintf("%s/%s", URL, release), err)
		}

		return nil
	}, 3)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("Failed to read body: %w", err)
	}

	// Builds are formatted in one of two ways:
	//   - <yyyy><mm><dd>.<build_number>
	//   - <yyyy><mm><dd>.n.<build_number>
	re := regexp.MustCompile(`\d{8}\.(n\.)?\d`)

	// Find all builds
	matches := re.FindAllString(string(content), -1)

	if len(matches) == 0 {
		return "", errors.New("Unable to find latest build")
	}

	// Sort builds
	sort.Strings(matches)

	// Return latest build
	return matches[len(matches)-1], nil
}
