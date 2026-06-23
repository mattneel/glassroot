package localrun

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
)

func createStaging(parent string) (string, error) {
	for i := 0; i < 16; i++ {
		var b [12]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", wrap(CodeStagingCreateFailed, "staging", "create random staging name", err)
		}
		p := filepath.Join(parent, ".glassroot-localrun-staging-"+hex.EncodeToString(b[:]))
		err := os.Mkdir(p, 0o700)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", wrap(CodeStagingCreateFailed, "staging", "create staging directory", err)
		}
		return p, nil
	}
	return "", errCode(CodeStagingCreateFailed, "staging", "staging collision limit exceeded", nil)
}

func ensureDir(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil {
		return err
	}
	return nil
}

func writeFileExclusive(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			_ = f.Close()
		}
	}()
	if len(data) > 0 {
		n, err := f.Write(data)
		if err != nil {
			return err
		}
		if n != len(data) {
			return io.ErrShortWrite
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func verifyFinalStagingTree(staging string) error {
	entries, err := os.ReadDir(staging)
	if err != nil {
		return wrap(CodePublishFailed, "publish", "read staging tree", err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return wrap(CodePublishFailed, "publish", "stat staging entry", err)
		}
		if info.Mode()&os.ModeType != 0 && !info.IsDir() {
			return errCode(CodePublishFailed, "publish", "staging contains unsupported entry type", nil)
		}
		if entry.Name() == "evidence" {
			if !info.IsDir() {
				return errCode(CodePublishFailed, "publish", "evidence output is not a directory", nil)
			}
			continue
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 != 0 {
			return errCode(CodePublishFailed, "publish", "staging file contract violated", nil)
		}
	}
	sort.Strings(got)
	want := []string{"evidence", "report.json", "report.md", "report.txt", "run.json"}
	if len(got) != len(want) {
		return errCode(CodePublishFailed, "publish", "staging output tree has unexpected entries", nil)
	}
	for i := range want {
		if got[i] != want[i] {
			return errCode(CodePublishFailed, "publish", "staging output tree has unexpected entries", nil)
		}
	}
	return nil
}

func removeAll(path string) error {
	if path == "" {
		return nil
	}
	return os.RemoveAll(path)
}
