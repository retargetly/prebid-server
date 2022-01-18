package idx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/template"

	"github.com/mxmCherry/openrtb/v15/openrtb2"
	"github.com/prebid/prebid-server/adapters"
	"github.com/prebid/prebid-server/config"
	"github.com/prebid/prebid-server/errortypes"
	"github.com/prebid/prebid-server/macros"
	"github.com/prebid/prebid-server/openrtb_ext"
)

type IDxAdapter struct {
	endpoint *template.Template
}

// Builder builds a new instance of the IDx adapter for the given bidder with the given config.
func Builder(bidderName openrtb_ext.BidderName, config config.Adapter) (adapters.Bidder, error) {
	template, err := template.New("endpointTemplate").Parse(config.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("unable to parse endpoint url template: %v", err)
	}

	bidder := &IDxAdapter{
		endpoint: template,
	}

	return bidder, nil
}

func (a *IDxAdapter) MakeRequests(request *openrtb2.BidRequest, requestInfo *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return nil, []error{err}
	}

	url, err := a.getUrl("")
	if err != nil {
		return nil, []error{err}
	}

	requestData := &adapters.RequestData{
		Method: "POST",
		Uri:    url,
		Body:   requestJSON,
	}

	return []*adapters.RequestData{requestData}, nil
}

func (a *IDxAdapter) MakeBids(request *openrtb2.BidRequest, requestData *adapters.RequestData, responseData *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	if responseData.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if responseData.StatusCode == http.StatusBadRequest {
		err := &errortypes.BadInput{
			Message: "Unexpected status code: 400. Bad request from publisher. Run with request.debug = 1 for more info.",
		}
		return nil, []error{err}
	}

	if responseData.StatusCode != http.StatusOK {
		err := &errortypes.BadServerResponse{
			Message: fmt.Sprintf("Unexpected status code: %d. Run with request.debug = 1 for more info.", responseData.StatusCode),
		}
		return nil, []error{err}
	}

	var response openrtb2.BidResponse
	if err := json.Unmarshal(responseData.Body, &response); err != nil {
		return nil, []error{err}
	}

	bidResponse := adapters.NewBidderResponseWithBidsCapacity(len(request.Imp))
	bidResponse.Currency = response.Cur
	for _, seatBid := range response.SeatBid {
		for i := range seatBid.Bid {
			price := strconv.FormatFloat(seatBid.Bid[i].Price, 'f', -1, 64)
			seatBid.Bid[i].AdM = strings.Replace(seatBid.Bid[i].AdM, "${AUCTION_PRICE}", price, -1)
			seatBid.Bid[i].NURL = strings.Replace(seatBid.Bid[i].NURL, "${AUCTION_PRICE}", price, -1)

			b := &adapters.TypedBid{
				Bid:     &seatBid.Bid[i],
				BidType: getMediaTypeForImp(seatBid.Bid[i].ImpID, request.Imp),
			}

			bidResponse.Bids = append(bidResponse.Bids, b)
		}
	}
	return bidResponse, nil
}

func getMediaTypeForImp(impID string, imps []openrtb2.Imp) openrtb_ext.BidType {
	mediaType := openrtb_ext.BidTypeBanner
	for _, imp := range imps {
		if imp.ID == impID {
			if imp.Banner == nil && imp.Video != nil {
				mediaType = openrtb_ext.BidTypeVideo
			} else if imp.Banner == nil && imp.Native != nil {
				mediaType = openrtb_ext.BidTypeNative
			}
		}
	}

	return mediaType
}

/*func (a *IDxAdapter) MakeBids(request *openrtb2.BidRequest, requestData *adapters.RequestData, responseData *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	fmt.Printf("\nStatus code: %d", responseData.StatusCode)
	if responseData.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if responseData.StatusCode != http.StatusOK {
		err := &errortypes.BadServerResponse{
			Message: fmt.Sprintf("Unexpected status code: %d. Body: %v.", responseData.StatusCode, responseData.Body),
		}
		return nil, []error{err}
	}

	response := openrtb2.BidResponse{
		ID:  request.ID,
		Cur: "USD",
		SeatBid: []openrtb2.SeatBid{
			{
				Bid: []openrtb2.Bid{
					{
						ID:    request.ID,
						ImpID: request.Imp[0].ID,
						CrID:  "banner:5",
						AdM:   string(responseData.Body),
						Price: 0.2,
					},
				},
			},
		},
	}

	bidsCapacity := len(request.Imp)
	bidResponse := adapters.NewBidderResponseWithBidsCapacity(bidsCapacity)
	bidResponse.Currency = response.Cur

	var errs []error
	for _, seatBid := range response.SeatBid {
		for _, bid := range seatBid.Bid {
			bidResponse.Bids = append(bidResponse.Bids, &adapters.TypedBid{
				Bid:     &bid,
				BidType: openrtb_ext.BidTypeBanner,
			})
		}
	}

	return bidResponse, errs
}*/

// getRequestHeaders returns the http headers to make the bid request
func getRequestHeaders() http.Header {
	headers := http.Header{}

	headers.Add("Content-Type", "application/json")
	headers.Add("Accept", "application/json")
	headers.Add("X-Openrtb-Version", "2.5")

	return headers
}

// getUrl returns the assembled url to hit the bidder
func (a *IDxAdapter) getUrl(host string) (string, error) {
	endpointParams := macros.EndpointTemplateParams{Host: host}
	uriString, errMacros := macros.ResolveMacros(a.endpoint, endpointParams)
	if errMacros != nil {
		return "", &errortypes.BadInput{
			Message: "Failed to resolve host macros",
		}
	}

	uri, errUrl := url.Parse(uriString)
	if errUrl != nil || uri.Scheme == "" || uri.Host == "" {
		return "", &errortypes.BadInput{
			Message: "Failed to create final URL with provided host",
		}
	}

	return uri.String(), nil
}
