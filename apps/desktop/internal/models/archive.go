package models

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Only the registered regular member is read, never an archive-provided path.
// The outer archive and final model are independently size/hash verified.
func downloadArchiveMember(ctx context.Context, client *http.Client, spec File, dir string, progress func(Progress)) error {
	archive := spec.Name + ".archive"
	input := File{Name: archive, Size: spec.ArchiveSize, SHA256: spec.ArchiveSHA256, URL: spec.URL}
	if input.Size <= 0 || input.SHA256 == "" {
		return errors.New("missing model archive digest")
	}
	if err := Download(ctx, client, Model{Files: []File{input}}, dir, func(p Progress) { progress(Progress{p.Done * spec.Size / input.Size, spec.Size, p.Stage}) }); err != nil {
		return err
	}
	path := filepath.Join(dir, archive)
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	reader := tar.NewReader(bzip2.NewReader(file))
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		header, err := reader.Next()
		if err != nil {
			return err
		}
		if header.Name != spec.ArchiveMember {
			continue
		}
		if !header.FileInfo().Mode().IsRegular() || header.Size != spec.Size {
			return errors.New("invalid model archive member")
		}
		final := filepath.Join(dir, spec.Name)
		partial := final + ".partial"
		out, err := os.OpenFile(partial, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		_, err = io.CopyN(out, reader, spec.Size)
		closeErr := out.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if err = verifyFile(ctx, partial, spec); err != nil {
			return err
		}
		return os.Rename(partial, final)
	}
}
