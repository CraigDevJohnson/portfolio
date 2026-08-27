package portal

// PortalPreviewFixture is the closed local-only visual fixture vocabulary.
type PortalPreviewFixture string

const (
	PortalPreviewFixtureNormal         PortalPreviewFixture = ""
	PortalPreviewFixtureEmpty          PortalPreviewFixture = "empty"
	PortalPreviewFixtureRetrievalError PortalPreviewFixture = "retrieval-error"
	PortalPreviewFixtureError          PortalPreviewFixture = "error"
)

func parsePortalPreviewFixture(raw string) (PortalPreviewFixture, bool) {
	fixture := PortalPreviewFixture(raw)
	switch fixture {
	case PortalPreviewFixtureNormal,
		PortalPreviewFixtureEmpty,
		PortalPreviewFixtureRetrievalError,
		PortalPreviewFixtureError:
		return fixture, true
	default:
		return PortalPreviewFixtureNormal, false
	}
}
