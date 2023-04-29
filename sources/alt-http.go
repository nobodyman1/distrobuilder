package sources

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/lxc/distrobuilder/shared"
)

type altLinux struct {
	common
}

func (s *altLinux) Run() error {
	arch := s.definition.Image.ArchitectureMapped

	if arch == "armhf" {
		arch = "armh"
	}

	baseURL := fmt.Sprintf(
		"%s/%s/cloud/%s/",
		s.definition.Source.URL,
		s.definition.Image.Release,
		arch,
	)
	fname := fmt.Sprintf("alt-%s-rootfs-systemd-%s.tar.xz", strings.ToLower(s.definition.Image.Release), arch)

	url, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("Failed to parse URL %q: %w", baseURL, err)
	}

	checksumFile := ""

	if !s.definition.Source.SkipVerification {
		if len(s.definition.Source.Keys) != 0 {
			checksumFile = baseURL + "SHA256SUMS"

			fpath, err := s.DownloadHash(s.definition.Image, checksumFile+".gpg", "", nil)
			if err != nil {
				return fmt.Errorf("Failed to download %q: %w", checksumFile+".gpg", err)
			}

			_, err = s.DownloadHash(s.definition.Image, checksumFile, "", nil)
			if err != nil {
				return fmt.Errorf("Failed to download %q: %w", checksumFile, err)
			}

			valid, err := s.VerifyFile(
				filepath.Join(fpath, "SHA256SUMS"),
				filepath.Join(fpath, "SHA256SUMS.gpg"))
			if err != nil {
				return fmt.Errorf("Failed to verify file: %w", err)
			}

			if !valid {
				return fmt.Errorf("Invalid signature for %q", "SHA256SUMS")
			}
		} else {
			// Force gpg checks when using http
			if url.Scheme != "https" {
				return errors.New("GPG keys are required if downloading from HTTP")
			}
		}
	}

	fpath, err := s.DownloadHash(s.definition.Image, baseURL+fname, checksumFile, sha256.New())
	if err != nil {
		return fmt.Errorf("Failed to download %q: %w", baseURL+fname, err)
	}

	s.logger.WithField("file", filepath.Join(fpath, fname)).Info("Unpacking image")

	// Unpack
	err = shared.Unpack(filepath.Join(fpath, fname), s.rootfsDir)
	if err != nil {
		return fmt.Errorf("Failed to unpack %q: %w", fname, err)
	}

	return nil
}
