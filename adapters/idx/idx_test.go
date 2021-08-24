package idx

import (
	"testing"

	"github.com/prebid/prebid-server/adapters/adapterstest"
	"github.com/prebid/prebid-server/config"
	"github.com/prebid/prebid-server/openrtb_ext"
)

func TestJsonSamples(t *testing.T) {
	bidder, buildErr := Builder(openrtb_ext.BidderIDx, config.Adapter{
		Endpoint: "http://{{.Host}}.smowtion.net/request/rtb",
	})

	if buildErr != nil {
		t.Fatalf("IDx Bidder returned unexpected error %v", buildErr)
	}

	adapterstest.RunJSONBidderTest(t, "idxtest", bidder)
}
