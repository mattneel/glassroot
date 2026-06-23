package config

import "testing"

func FuzzParseAndValidate(f *testing.F) {
	f.Add(readFixture(f, "valid/pipeline.yaml"))
	f.Add([]byte{})
	f.Add(readFixture(f, "invalid/duplicate-key.yaml"))
	f.Add(readFixture(f, "invalid/unknown-field.yaml"))
	f.Add(readFixture(f, "invalid/alias.yaml"))
	f.Add(readFixture(f, "invalid/multiple-documents.yaml"))
	f.Add(readFixture(f, "invalid/invalid-unit.yaml"))
	f.Add(readFixture(f, "invalid/invalid-path.yaml"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, err := ParseAndValidate(data)
		assertBoundedSanitizedError(t, err)
	})
}
