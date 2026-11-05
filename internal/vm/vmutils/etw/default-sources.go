package etw

// defaultLogSourcesInfo defines the list of trusted ETW providers
var defaultLogSourcesInfo = LogSourcesInfo{
	LogConfig: LogConfig{
		Sources: []Source{
			{
				Type: "ETW",
				Providers: []EtwProvider{
					{
						ProviderName: SidecarProviderName,
						Level:        "Information",
					},
					{
						ProviderName: "microsoft.windows.hyperv.compute",
						Level:        "Information",
					},
					{
						ProviderName: "microsoft-windows-guest-network-service",
						Level:        "Information",
					},
					{
						ProviderName: "microsoft.windows.filesystem.cimfs",
						Level:        "Information",
					},
					{
						ProviderName: "microsoft.windows.filesystem.unionfs",
						Level:        "Information",
					},
					{
						ProviderName: "microsoft-windows-bitlocker-driver",
						Level:        "Information",
					},
					{
						ProviderName: "microsoft-windows-bitlocker-api",
						Level:        "Information",
					},
					{
						ProviderName: "microsoft.windows.security.keyguard",
						Level:        "Information",
					},
					{
						ProviderName: "microsoft.windows.security.keyguard.attestation.verify",
						Level:        "Information",
					},
					{
						ProviderName: "microsoft.windows.containers.setup",
						Level:        "Information",
					},
					{
						ProviderName: "microsoft.windows.containers.storage",
						Level:        "Information",
					},
					{
						ProviderName: "microsoft.windows.containers.library",
						Level:        "Information",
					},
					{
						ProviderName: "microsoft.windows.containers.dynamicimage",
						Level:        "Information",
					},
					{
						ProviderName: "microsoft.windows.logforwardservice.provider",
						Level:        "Information",
					},
				},
			},
		},
	},
}
