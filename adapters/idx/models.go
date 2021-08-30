package idx

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mxmCherry/openrtb/v15/openrtb2"
	"github.com/prebid/prebid-server/openrtb_ext"
)

type idxUserExtEidUidExt struct {
	RtiPartner string `json:"rtiPartner,omitempty"`
}

// source - rtiPartner
var enabledExtendedUserIds = map[string]string{
	"idx.lat": "idx",
}

func getEnabledUserIds(request *openrtb2.BidRequest) (*openrtb_ext.ExtUser, []error) {
	var errs []error

	if request.User != nil && request.User.Ext != nil {
		var extUser openrtb_ext.ExtUser
		if err := json.Unmarshal(request.User.Ext, &extUser); err != nil {
			return nil, errs
		}

		eids := make([]openrtb_ext.ExtUserEid, 0)

		for _, eid := range extUser.Eids {
			eidSource := strings.ToLower(eid.Source)

			if _, ok := enabledExtendedUserIds[eidSource]; ok {
				if len(eid.Uids) == 0 {
					errs = append(errs, fmt.Errorf("UserId: %s : invalid uids length ", eidSource))
					continue
				}

				uid := eid.Uids[0]
				if uid.ID == "" {
					errs = append(errs, fmt.Errorf("UserId: %s : invalid ID %s", eidSource, uid.ID))
					continue
				}

				var eidUidExt idxUserExtEidUidExt
				if err := json.Unmarshal(uid.Ext, &eidUidExt); err != nil {
					fmt.Println("errored")
					errs = append(errs, err)
					continue
				}

				if enabledExtendedUserIds[eid.Source] != eidUidExt.RtiPartner {
					errs = append(errs, fmt.Errorf("UserId: %s : RtiPartner mismatch: expected %s got %s", eidSource, enabledExtendedUserIds[eidSource], eidUidExt.RtiPartner))
					continue
				}

				eids = append(eids, eid)
			}
		}

		extUser.Eids = eids

		return &extUser, errs
	}

	return nil, errs
}
