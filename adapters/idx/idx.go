package idx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	impressions := request.Imp
	result := make([]*adapters.RequestData, 0, len(impressions))
	errs := make([]error, 0, len(impressions))

	headers := getRequestHeaders()

	for _, impression := range impressions {
		if impression.Banner != nil {
			banner := impression.Banner
			if banner.W == nil && banner.H == nil {
				if banner.Format == nil {
					errs = append(errs, &errortypes.BadInput{
						Message: "Impression with id: " + impression.ID + " has following error: Banner width and height is not provided and banner format is missing. At least one is required",
					})
					continue
				}
				if len(banner.Format) == 0 {
					errs = append(errs, &errortypes.BadInput{
						Message: "Impression with id: " + impression.ID + " has following error: Banner width and height is not provided and banner format array is empty. At least one is required",
					})
					continue
				}
			}

		}

		// Parse bidder extra params
		var bidderExt adapters.ExtImpBidder
		err := json.Unmarshal(impression.Ext, &bidderExt)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		// Parse IDx params present on bidder extra params
		var impressionExt openrtb_ext.ExtImpIDx
		err = json.Unmarshal(bidderExt.Bidder, &impressionExt)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		// Get url using impression host
		url, err := a.getUrl(impressionExt.Host)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		impressionExt.Host = ""
		idxExtReq, err := json.Marshal(impressionExt)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		impression.Ext = nil
		request.Ext = idxExtReq

		// Get enabled extended ids
		extUser, errsExtUser := getEnabledUserIds(request)
		if errsExtUser != nil {
			errs = append(errs, errsExtUser...)
		}

		extUserBody, err := json.Marshal(&extUser)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		request.User.Ext = extUserBody

		request.Imp = []openrtb2.Imp{impression}
		body, err := json.Marshal(request)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		result = append(result, &adapters.RequestData{
			Method:  "POST",
			Uri:     url,
			Body:    body,
			Headers: headers,
		})
	}

	request.Imp = impressions

	return result, errs
}

func (a *IDxAdapter) MakeBids(request *openrtb2.BidRequest, requestData *adapters.RequestData, responseData *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	if responseData.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if responseData.StatusCode != http.StatusOK {
		err := &errortypes.BadServerResponse{
			Message: fmt.Sprintf("Unexpected status code: %d. Body: %v.", responseData.StatusCode, responseData.Body),
		}
		return nil, []error{err}
	}

	var response openrtb2.BidResponse
	if err := json.Unmarshal(responseData.Body, &response); err != nil {
		return nil, []error{err}
	}

	bidsCapacity := len(request.Imp)
	bidResponse := adapters.NewBidderResponseWithBidsCapacity(bidsCapacity)
	bidResponse.Currency = response.Cur

	var errs []error
	for _, seatBid := range response.SeatBid {
		for _, bid := range seatBid.Bid {
			fmt.Println("bid")
			bid := bid
			bidType := getMediaTypeForImp(bid.ImpID, request.Imp)
			if bidType == nil {
				errs = append(errs, &errortypes.BadServerResponse{
					Message: "ignoring bid id=" + bid.ID + ", request doesn't contain any valid impression with id=" + bid.ImpID,
				})
				continue
			}

			bidResponse.Bids = append(bidResponse.Bids, &adapters.TypedBid{
				Bid:     &bid,
				BidType: *bidType,
			})
		}
	}

	return bidResponse, errs
}

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

func getMediaTypeForImp(impID string, imps []openrtb2.Imp) *openrtb_ext.BidType {
	mediaType := openrtb_ext.BidTypeBanner

	for _, imp := range imps {
		if imp.ID == impID {
			if imp.Banner == nil && imp.Video != nil {
				mediaType = openrtb_ext.BidTypeVideo
			}

			return &mediaType
		}
	}

	return nil
}
