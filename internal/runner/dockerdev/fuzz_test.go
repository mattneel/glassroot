package dockerdev

import "testing"

func FuzzValidateDockerDevWorkspace(f *testing.F) {
	f.Add("att-base-test-r1", "/tmp/glassroot-workspace")
	f.Add("", "relative")
	f.Fuzz(func(t *testing.T, id, path string) {
		_ = validateWorkspaceBindingSyntax(WorkspaceBinding{AttemptID: id, HostPath: path})
	})
}

func FuzzBuildContainerConfiguration(f *testing.F) {
	f.Add("/bin/sh", "echo hi", int64(1), int64(64<<20), int64(32))
	f.Add("/bin/sh", "$(rm -rf /)", int64(999999999999), int64(-1), int64(-1))
	f.Fuzz(func(t *testing.T, shell, run string, cpu, mem, pids int64) {
		_, _ = buildContainerSpecForFuzz(shell, run, cpu, mem, pids)
	})
}
