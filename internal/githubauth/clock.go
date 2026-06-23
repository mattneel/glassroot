package githubauth

import "time"

type Clock interface{ Now() time.Time }
