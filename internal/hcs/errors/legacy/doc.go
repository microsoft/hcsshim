// This package contains the legacy HcsError struct exposed to external clients.
//
// Ideally, this would be moved to the root, ie. `/errors.go`.
// However `errors.go` imports  ["internal/hns".EndpointNotFoundError] and
// ["internal/hns".NetworkNotFoundError], and ["internal.hns"],
// (via `hnsfuncs.go`), calls [New], creating a cycle.
package legacy // hcserrors "github.com/Microsoft/hcsshim/internal/hcs/errors/legacy"
