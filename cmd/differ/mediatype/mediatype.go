// This package deals with media types and extensions specific to Windows containers (LCOW and WCOW).
package mediatype

const (
	ExtensionIsolated = "isolated"
	ExtensionLCOW     = "lcow"
	ExtensionWCOW     = "wcow"

	MediaTypeMicrosoftBase              = "application/vnd.microsoft"
	MediaTypeMicrosoftImageLayerVHD     = "application/vnd.microsoft.image.layer.v1.vhd"
	MediaTypeMicrosoftImageLayerExt4    = "application/vnd.microsoft.image.layer.v1.vhd+ext4"
	MediaTypeMicrosoftImageLayerWCLayer = "application/vnd.microsoft.image.layer.v1.vhd+wclayer"
)
