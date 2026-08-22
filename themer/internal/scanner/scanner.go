package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ScannedFile represents an HTML file discovered during scanning
type ScannedFile struct {
	Path         string    `json:"path"`
	RelativePath string    `json:"relative_path"`
	Filename     string    `json:"filename"`
	SizeBytes    int64     `json:"size_bytes"`
	ModifiedAt   time.Time `json:"modified_at"`
	ParentDir    string    `json:"parent_dir"` // immediate parent folder name
}

// ScanDirectory recursively walks a directory and finds all .html files
func ScanDirectory(rootPath string) ([]ScannedFile, error) {
	rootPath, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(rootPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &os.PathError{Op: "scan", Path: rootPath, Err: os.ErrInvalid}
	}

	var results []ScannedFile
	err = filepath.Walk(rootPath, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors, keep walking
		}
		if fi.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(fi.Name()))
		if ext != ".html" && ext != ".htm" {
			return nil
		}

		relPath, _ := filepath.Rel(rootPath, path)
		results = append(results, ScannedFile{
			Path:         filepath.ToSlash(path),
			RelativePath: filepath.ToSlash(relPath),
			Filename:     fi.Name(),
			SizeBytes:    fi.Size(),
			ModifiedAt:   fi.ModTime(),
			ParentDir:    filepath.Base(filepath.Dir(path)),
		})
		return nil
	})
	return results, err
}
