//go:build windows

package payload

import (
	"fmt"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/typeurl"
	"github.com/gogo/protobuf/types"
)

func init() {
	typeurl.Register(&WCLayerImportOptions{},
		"github.com/Microsoft/hcsshim/cmd/differ/payload", "WCLayerImportOptions")
}

type WCLayerImportOptions struct {
	RootPath string
	Parents  []string
}

func (o *WCLayerImportOptions) ToAny() (*types.Any, error) {
	a, err := typeurl.MarshalAny(o)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Tar2Ext4Options: %w", err)
	}
	return a, nil
}

func (o *WCLayerImportOptions) FromAny(a *types.Any) error {
	v, err := typeurl.UnmarshalAny(a)
	if err != nil || v == nil {
		return fmt.Errorf("unmarshal WCLayerImportOptions: %w", err)
	}

	oo, ok := v.(*WCLayerImportOptions)
	if !ok {
		return fmt.Errorf("payload type is %T, not WCLayerImportOptions: %w", v, errdefs.ErrInvalidArgument)
	}
	*o = *oo
	return nil
}
