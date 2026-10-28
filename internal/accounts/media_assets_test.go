package accounts

import "testing"

func TestMediaAssetActionsUseTheExistingAdminSessionAndExplicitAllowlist(t *testing.T) {
	s := fixture(t)
	_, _, actor, e := s.login("admin", testPassword, false)
	if e != nil {
		t.Fatal(e)
	}
	for _, action := range []string{"media_asset_import", "media_asset_delete"} {
		called := false
		if e = s.ChannelChange(actor, action, func() error { called = true; return nil }); e != nil || !called {
			t.Fatal("media asset action rejected", e)
		}
	}
	for _, action := range []string{"media_asset_anything", "media_asset_https://synthetic.invalid/private"} {
		if e = s.ChannelChange(actor, action, func() error { t.Fatal("arbitrary action executed"); return nil }); e == nil {
			t.Fatal("arbitrary asset action accepted")
		}
	}
	viewer := actor
	viewer.Role = "viewer"
	if s.ChannelChange(viewer, "media_asset_import", func() error { t.Fatal("viewer imported an asset"); return nil }) == nil {
		t.Fatal("viewer allowed")
	}
}
