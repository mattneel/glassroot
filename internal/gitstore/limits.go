package gitstore

import "time"

const (
	MaxRevisionSelectorBytes        = 1 << 10
	MaxRepositoryPathBytes          = 4 << 10
	MaxGitConfigBytes               = 1 << 20
	MaxGitStdoutBytes               = 1 << 20
	MaxGitStderrBytes               = 64 << 10
	MaxTreeListingBytes             = 64 << 20
	MaxTreeEntries                  = 100000
	MaxTreeDepth                    = 128
	MaxTreePathBytes                = 4096
	MaxTreePathComponentBytes       = 255
	MaxBlobBytes              int64 = 128 << 20
)

const DefaultGitCommandTimeout = 30 * time.Second
