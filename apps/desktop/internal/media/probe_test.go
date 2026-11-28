package media

import "testing"

func TestHasVideoExcludesAudioCoverArt(t *testing.T) {
	cover := Stream{CodecType: "video", CodecName: "mjpeg"}
	cover.Disposition.AttachedPic = 1
	info := Info{Streams: []Stream{{CodecType: "audio"}, cover}}
	if info.HasVideo() {
		t.Fatal("audio cover art must not enable video composition or video pairing")
	}
	info.Streams = append(info.Streams, Stream{CodecType: "video", CodecName: "h264"})
	if !info.HasVideo() {
		t.Fatal("a real video stream was ignored after cover art")
	}
}
