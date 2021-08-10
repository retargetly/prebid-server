package firstpartydata

import (
	"encoding/json"
	"errors"
	"github.com/buger/jsonparser"
	"github.com/evanphx/json-patch"
	"github.com/mxmCherry/openrtb/v15/openrtb2"
	"github.com/prebid/prebid-server/openrtb_ext"
	"github.com/prebid/prebid-server/util/jsonutil"
)

const (
	site = "site"
	app  = "app"
	user = "user"
	data = "data"
)

func GetFPDData(request []byte) ([]byte, map[string][]byte, error) {
	//If {site,app,user}.data exists, merge it into {site,app,user}.ext.data and remove {site,app,user}.data

	fpdReqData := make(map[string][]byte, 0)
	request, siteFPD, err := jsonutil.FindAndDropElement(request, site, data)
	if err != nil {
		return request, nil, err
	}
	fpdReqData[site] = siteFPD

	request, appFPD, err := jsonutil.FindAndDropElement(request, app, data)
	if err != nil {
		return request, nil, err
	}
	fpdReqData[app] = appFPD

	fpdReqData[user] = []byte{}
	userDataBytes, _, _, err := jsonparser.Get(request, user, data)
	if err != nil && err != jsonparser.KeyPathNotFoundError {
		return request, nil, err
	}

	if len(userDataBytes) > 0 {
		var userData []openrtb2.Data
		userDataCopy := make([]byte, len(userDataBytes))
		copy(userDataCopy, userDataBytes)
		err = json.Unmarshal(userDataCopy, &userData)
		if err != nil {
			//unable to unmarshal to []openrtb2.Data, meaning this is FPD data
			request, err = jsonutil.DropElement(request, user, data)
			if err != nil {
				return request, nil, err
			}
			fpdReqData[user] = userDataCopy
		}
	}

	return request, fpdReqData, nil
}

func BuildFPD(bidRequest *openrtb2.BidRequest, fpdBidderData map[openrtb_ext.BidderName]*openrtb_ext.FPDData, firstPartyData map[string][]byte) (map[openrtb_ext.BidderName]*openrtb_ext.FPDData, []error) {

	// If an attribute doesn't pass defined validation checks,
	// it should be removed from the request with a warning placed
	// in the messages section of debug output
	// The auction should continue

	errL := make([]error, 0)
	resolvedFpdData := make(map[openrtb_ext.BidderName]*openrtb_ext.FPDData)

	for bidderName, fpdConfig := range fpdBidderData {

		resolvedFpdConfig := &openrtb_ext.FPDData{}

		if fpdConfig.User != nil {
			if bidRequest.User == nil {
				resolvedFpdConfig.User = fpdConfig.User
			} else {
				resUser, err := mergeFPD(bidRequest.User, fpdConfig.User, firstPartyData, "user")
				if err != nil {
					errL = append(errL, err)
					return nil, errL
				}
				newUser := &openrtb2.User{}
				err = json.Unmarshal(resUser, newUser)
				if err != nil {
					errL = append(errL, err)
					return nil, errL
				}

				resolvedFpdConfig.User = newUser
			}
		}

		if fpdConfig.App != nil {
			if bidRequest.App == nil {
				resolvedFpdConfig.App = fpdConfig.App
			} else {
				resApp, err := mergeFPD(bidRequest.App, fpdConfig.App, firstPartyData, "app")
				if err != nil {
					errL = append(errL, err)
					return nil, errL
				}

				newApp := &openrtb2.App{}
				err = json.Unmarshal(resApp, newApp)
				if err != nil {
					errL = append(errL, err)
					return nil, errL
				}

				resolvedFpdConfig.App = newApp
			}
		}

		if fpdConfig.Site != nil {
			if bidRequest.Site == nil {
				resolvedFpdConfig.Site = fpdConfig.Site
			} else {
				resSite, err := mergeFPD(bidRequest.Site, fpdConfig.Site, firstPartyData, "site")
				if err != nil {
					errL = append(errL, err)
					return nil, errL
				}

				newSite := &openrtb2.Site{}
				err = json.Unmarshal(resSite, newSite)
				if err != nil {
					errL = append(errL, err)
					return nil, errL
				}

				resolvedFpdConfig.Site = newSite
			}
		}
		resolvedFpdData[bidderName] = resolvedFpdConfig
	}

	return resolvedFpdData, errL
}

func mergeFPD(input interface{}, fpd interface{}, data map[string][]byte, value string) ([]byte, error) {

	inputByte, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	fpdByte, err := json.Marshal(fpd)
	if err != nil {
		return nil, err
	}
	resultMerged, err := jsonpatch.MergePatch(inputByte, fpdByte)
	if err != nil {
		return nil, err
	}

	//If {site,app,user}.data exists, merge it into {site,app,user}.ext.data
	//Question: if fpdSite.Ext.data exists should it be overwritten with Site.data?
	if len(data[value]) > 0 {
		extData := buildExtData(data[value])
		return jsonpatch.MergePatch(resultMerged, extData)
	}

	return resultMerged, err
}

func buildExtData(data []byte) []byte {
	res := []byte(`{"ext":{"data":`)
	res = append(res, data...)
	res = append(res, []byte(`}}`)...)
	return res
}

func PreprocessFPD(reqExtPrebid openrtb_ext.ExtRequestPrebid) (map[openrtb_ext.BidderName]*openrtb_ext.FPDData, openrtb_ext.ExtRequestPrebid) {
	//map to store bidder configs to process
	fpdData := make(map[openrtb_ext.BidderName]*openrtb_ext.FPDData)

	if reqExtPrebid.Data != nil && len(reqExtPrebid.Data.Bidders) != 0 && reqExtPrebid.BidderConfigs != nil {

		//every entry in ext.prebid.bidderconfig[].bidders would also need to be in ext.prebid.data.bidders or it will be ignored
		bidderTable := make(map[string]bool) //boolean just to check existence of the element in map
		for _, bidder := range reqExtPrebid.Data.Bidders {
			bidderTable[bidder] = true
		}

		for _, bidderConfig := range *reqExtPrebid.BidderConfigs {
			for _, bidder := range bidderConfig.Bidders {
				if bidderTable[bidder] {

					if fpdData[openrtb_ext.BidderName(bidder)] == nil {
						fpdData[openrtb_ext.BidderName(bidder)] = bidderConfig.FPDConfig.FPDData
					} else {
						//this will overwrite previously set site/app/user.
						//Last defined bidder-specific config will take precedence
						//Do we need to check it?
						fpdBidderData := fpdData[openrtb_ext.BidderName(bidder)]
						if bidderConfig.FPDConfig.FPDData.Site != nil {
							fpdBidderData.Site = bidderConfig.FPDConfig.FPDData.Site
						}
						if bidderConfig.FPDConfig.FPDData.App != nil {
							fpdBidderData.App = bidderConfig.FPDConfig.FPDData.App
						}
						if bidderConfig.FPDConfig.FPDData.User != nil {
							fpdBidderData.User = bidderConfig.FPDConfig.FPDData.User
						}
					}
				}
			}
		}
	}

	reqExtPrebid.BidderConfigs = nil
	if reqExtPrebid.Data != nil {
		reqExtPrebid.Data.Bidders = nil
	}

	return fpdData, reqExtPrebid
}

func ValidateFPDConfig(reqExtPrebid openrtb_ext.ExtRequestPrebid) error {

	//Both FPD global and bidder specific permissions are specified
	if reqExtPrebid.Data == nil && reqExtPrebid.BidderConfigs == nil {
		return nil
	}

	if reqExtPrebid.Data != nil && len(reqExtPrebid.Data.Bidders) != 0 && reqExtPrebid.BidderConfigs == nil {
		return errors.New(`request.ext.prebid.data.bidders are specified but reqExtPrebid.BidderConfigs are not`)
	}
	if reqExtPrebid.Data != nil && len(reqExtPrebid.Data.Bidders) == 0 && reqExtPrebid.BidderConfigs != nil {
		return errors.New(`request.ext.prebid.data.bidders are not specified but reqExtPrebid.BidderConfigs are`)
	}

	if reqExtPrebid.Data == nil && reqExtPrebid.BidderConfigs != nil {
		return errors.New(`request.ext.prebid.data is not specified but reqExtPrebid.BidderConfigs are`)
	}

	return nil
}
