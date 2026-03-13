package etw

// defaultLogSourcesInfo is the native Go representation of the default-logsources.json file.
var defaultLogSourcesInfo = LogSourcesInfo{
	LogConfig: LogConfig{
		Sources: []Source{
			{
				Type: "ETW",
				Providers: []EtwProvider{
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
