package hls

import "testing"

func TestAuthorizedHLSValidatesTheTracksThatWillActuallyBeMapped(t *testing.T) {
	valid := []byte(`{"streams":[{"codec_type":"video","codec_name":"h264"},{"codec_type":"audio","codec_name":"aac"},{"codec_type":"audio","codec_name":"ac3"},{"codec_type":"subtitle","codec_name":"dvb_subtitle"}]}`)
	if validateSelectedCodecs(valid) != nil {
		t.Fatal("compatible selected tracks rejected")
	}
	for _, bad := range []string{`{"streams":[{"codec_type":"video","codec_name":"mpeg2video"},{"codec_type":"audio","codec_name":"aac"}]}`, `{"streams":[{"codec_type":"video","codec_name":"h264"},{"codec_type":"audio","codec_name":"ac3"},{"codec_type":"audio","codec_name":"aac"}]}`, `{"streams":[]}`} {
		if validateSelectedCodecs([]byte(bad)) == nil {
			t.Fatal("unsupported selected tracks accepted")
		}
	}
}
