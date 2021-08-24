package openrtb_ext

// ExtImpIDx defines the contract for bidrequest.imp[i].ext.idx
type ExtImpIDx struct {
	Host      string `json:"host,omitempty"`
	AdSpaceId string `json:"ad_space_id,omitempty"`
	SellerId  string `json:"seller_id,omitempty"`
	SspId     string `json:"ssp_id,omitempty"`
}
