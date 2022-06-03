package payload

import "github.com/gogo/protobuf/types"

type FromAny interface {
	FromAny(a *types.Any) error
}
