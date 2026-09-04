package plugins

import (
	"context"

	"github.com/davidadel66/evie/internal/tools"
)

const (
	YouTubePluginID                  PluginID     = "youtube"
	YouTubeTranscriptCapabilityID    CapabilityID = "youtube.transcript"
	YouTubeScrapeChannelCapabilityID CapabilityID = "youtube.scrape_channel"
	youtubeImplementationVersion                  = "1.0.0"
	youtubeContractVersion                        = "1.0.0"
)

type YouTube struct{}

func NewYouTube() YouTube { return YouTube{} }

func (YouTube) Start(context.Context) error { return nil }

func (YouTube) Stop(context.Context) error { return nil }

func (YouTube) Manifest() Manifest {
	return Manifest{
		ID:                    YouTubePluginID,
		ImplementationVersion: youtubeImplementationVersion,
		KernelCompatibility: VersionRange{
			Minimum: KernelAPIVersion, MaximumExclusive: "2.0.0",
		},
		Capabilities: []CapabilityContract{
			{ID: YouTubeTranscriptCapabilityID, Version: youtubeContractVersion},
			{ID: YouTubeScrapeChannelCapabilityID, Version: youtubeContractVersion},
		},
	}
}

func (YouTube) ToolCapabilities() []ToolCapability {
	youtubeTools := tools.YouTubeTools()
	return []ToolCapability{
		{
			ID: YouTubeTranscriptCapabilityID, ContractVersion: youtubeContractVersion,
			Tool: youtubeTools[0],
		},
		{
			ID: YouTubeScrapeChannelCapabilityID, ContractVersion: youtubeContractVersion,
			Tool: youtubeTools[1],
		},
	}
}
