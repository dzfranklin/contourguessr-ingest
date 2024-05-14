package flickrindexer

import (
	"bytes"
	"context"
	"contourguessr-ingest/repos"
	_ "embed"
	"encoding/json"
	"github.com/minio/minio-go/v7"
	"github.com/paulmach/orb/encoding/wkt"
	"github.com/stretchr/testify/assert"
	"io"
	"sync"
	"testing"
	"time"
)

var denali = makeRegion("MULTIPOLYGON(((-148.9802346 63.4235725,-148.8146541 63.4616369,-148.8444543 63.5867466,-148.7955802 63.6309029,-148.83931 63.6536962,-148.7778228 63.6787532,-148.8750363 63.7048778,-148.9318994 63.7987494,-149.7999658 63.8240715,-149.7999768 63.9106903,-149.2116761 63.9107019,-149.2116852 63.9973187,-151.1847691 63.9942729,-151.4478868 63.8638944,-151.6857672 63.8921945,-151.84255 63.808931,-151.8161025 63.7903894,-151.8675798 63.7538597,-151.8403452 63.7263906,-151.9292177 63.6617517,-151.8532938 63.4632989,-152.0513717 63.4140767,-152.0718655 63.3590078,-152.2013909 63.3404784,-152.1163683 63.2808507,-152.4281151 63.2356693,-152.4280814 62.9577432,-152.1132035 62.9577515,-152.0816928 62.7908229,-151.9733012 62.7107146,-151.7392633 62.7290805,-151.6050384 62.6295154,-151.5922093 62.5564355,-151.4003865 62.5526117,-151.4003832 62.4659755,-151.1540858 62.4659824,-151.1544562 62.5526186,-150.9865237 62.6392583,-150.2979555 62.6392774,-150.2979503 62.8125452,-150.1102692 62.8125515,-150.1191133 62.9578118,-150.0349038 62.9578147,-150.0094927 63.1214032,-149.6530328 63.2424734,-149.3620232 63.2177177,-149.3620607 63.3015148,-148.9850795 63.382981,-148.9802346 63.4235725)))")

type Expect struct {
	cursor repos.Cursor
	photos []repos.Photo
	err    error
}

func TestStep(t *testing.T) {
	table := []struct {
		name   string
		region repos.Region
		cursor repos.Cursor
		fc     *flickrMock
		mc     *mcMock
		expect Expect
	}{
		{name: "start", region: denali,
			fc: mockFC(t).expectCall(
				"flickr.photos.search",
				map[string]string{
					"bbox":            "-152.428115,62.465975,-148.777823,63.997319",
					"min_upload_date": "0",
					"sort":            "date-posted-asc",
					"safe_search":     "1",
					"content_type":    "1",
					"license":         "1,2,3,4,5,6,7,8,9,10",
					"extras":          "geo,date_upload",
					"per_page":        "100",
					"page":            "1",
				},
				[]byte(`{
				  "photos": {
					"photo": [
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" }
				    ]
				  }
				}`),
				nil,
			).expectCall(
				"flickr.photos.getInfo",
				map[string]string{"photo_id": "34742333", "extras": "sizes"},
				[]byte(`{
				  "photo": {
					"id": "34742333",
					"sizes": {
					  "size": [
						{
						  "label": "Square",
						  "width": 75,
						  "height": 75,
						  "source": "https://fake/square.jpg",
						  "media": "photo"
						},
						{
						  "label": "Thumbnail",
						  "width": 100,
						  "height": 80,
						  "source": "https://fake/thumbnail.jpg",
						  "media": "photo"
						},
						{
						  "label": "Medium",
						  "width": 500,
						  "height": 400,
						  "source": "https://fake/medium.jpg",
						  "media": "photo"
						},
						{
						  "label": "Large",
						  "width": 1024,
						  "height": 819,
						  "source": "https://fake/large.jpg",
						  "media": "photo"
						},
						{
						  "label": "Original",
						  "width": 1280,
						  "height": 1024,
						  "source": "https://fake/original.jpg",
						  "media": "photo"
						}
					  ]
					}
				  }
				}`),
				nil,
			).expectCall(
				"flickr.photos.getExif",
				map[string]string{"photo_id": "34742333"},
				[]byte(`{
				  "photo": {
					"exif": [
					  {"tagspace": "IFD0", "tagspaceid": 0, "tag": "Make", "label": "Make", "raw": {"_content": "Canon"}}
					]
				  }
				}`),
				nil,
			).expectDl(
				"https://fake/medium.jpg",
				[]byte("fake medium image"),
				nil,
			).expectDl(
				"https://fake/large.jpg",
				[]byte("fake large image"),
				nil,
			),
			mc: mockMC(t).expectFPutObject(
				"contourguessr-photos",
				"flickr/34742333/medium.jpg",
				minio.PutObjectOptions{ContentType: "image/jpeg"},
				minio.UploadInfo{},
				nil,
			).expectFPutObject(
				"contourguessr-photos",
				"flickr/34742333/large.jpg",
				minio.PutObjectOptions{ContentType: "image/jpeg"},
				minio.UploadInfo{},
				nil,
			),
			expect: Expect{
				cursor: repos.Cursor{MinUploadDate: time.Unix(0, 0), Page: 1},
				photos: []repos.Photo{
					{
						RegionId: 42,
						Id:       "34742333",
						Info: json.RawMessage(`{
					  		"id": "34742333"
						}`),
						Sizes: json.RawMessage(`[
							{"label": "Square", "width": 75, "height": 75, "source": "https://fake/square.jpg", "media": "photo"},
							{"label": "Thumbnail", "width": 100, "height": 80, "source": "https://fake/thumbnail.jpg", "media": "photo"},
							{"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"},
							{"label": "Large", "width": 1024, "height": 819, "source": "https://fake/large.jpg", "media": "photo"},
							{"label": "Original", "width": 1280, "height": 1024, "source": "https://fake/original.jpg", "media": "photo"}
					  	]`),
						Exif: json.RawMessage(`[
							{"tagspace": "IFD0", "tagspaceid": 0, "tag": "Make", "label": "Make", "raw": {"_content": "Canon"}}
					  	]`),
					},
				},
			},
		},
		{name: "advance min upload date", region: denali,
			cursor: repos.Cursor{MinUploadDate: time.Unix(0, 0), Page: 20},
			fc: mockFC(t).expectCall(
				"flickr.photos.search",
				map[string]string{
					"bbox":            "-152.428115,62.465975,-148.777823,63.997319",
					"min_upload_date": "0",
					"sort":            "date-posted-asc",
					"safe_search":     "1",
					"content_type":    "1",
					"license":         "1,2,3,4,5,6,7,8,9,10",
					"extras":          "geo,date_upload",
					"per_page":        "100",
					"page":            "20",
				},
				[]byte(`{
				  "photos": {
					"photo": [
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" },
					  { "id": "34742333", "owner": "39916636@N00", "license": "5", "latitude": "63.519358", "longitude": "-150.044398", "dateupload": "1151810509" }
					]
				  }
				}`),
				nil,
			).expectManyCall(
				"flickr.photos.getInfo",
				[]byte(`{"photo": {
				  "sizes":
				    {
				      "size": [
						{"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"}
 				      ]
                    }
                  }
                }`),
				nil,
			).expectManyCall(
				"flickr.photos.getExif",
				[]byte(`{"photo": {"exif": []}}`),
				nil,
			).expectManyDl(
				[]byte("fake medium image"), nil,
			),
			mc: mockMC(t).noop(),
			expect: Expect{
				cursor: repos.Cursor{MinUploadDate: time.Unix(1151810509, 0), Page: 1},
				photos: []repos.Photo{
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
					{Id: "34742333", RegionId: 42, Info: json.RawMessage(`{}`), Sizes: json.RawMessage(`[ {"label": "Medium", "width": 500, "height": 400, "source": "https://fake/medium.jpg", "media": "photo"} ]`), Exif: json.RawMessage(`[]`)},
				},
			},
		},
		{name: "empty page preserves cursor", region: denali,
			cursor: repos.Cursor{MinUploadDate: time.Unix(42, 0), Page: 4},
			fc: mockFC(t).expectCall(
				"flickr.photos.search",
				map[string]string{
					"bbox":            "-152.428115,62.465975,-148.777823,63.997319",
					"min_upload_date": "42",
					"sort":            "date-posted-asc",
					"safe_search":     "1",
					"content_type":    "1",
					"license":         "1,2,3,4,5,6,7,8,9,10",
					"extras":          "geo,date_upload",
					"per_page":        "100",
					"page":            "4",
				},
				[]byte(`{
				  "photos": {
					"photo": []
				  }
				}`),
				nil,
			),
			expect: Expect{
				cursor: repos.Cursor{MinUploadDate: time.Unix(42, 0), Page: 4},
				photos: nil,
			},
		},
		{name: "empty page at cutoff preserves cursor", region: denali, cursor: repos.Cursor{MinUploadDate: time.Unix(42, 0), Page: 20},
			fc: mockFC(t).expectCall(
				"flickr.photos.search",
				map[string]string{
					"bbox":            "-152.428115,62.465975,-148.777823,63.997319",
					"min_upload_date": "42",
					"sort":            "date-posted-asc",
					"safe_search":     "1",
					"content_type":    "1",
					"license":         "1,2,3,4,5,6,7,8,9,10",
					"extras":          "geo,date_upload",
					"per_page":        "100",
					"page":            "20",
				},
				[]byte(`{
				  "photos": {
					"photo": []
				  }
				}`),
				nil,
			),
			expect: Expect{
				cursor: repos.Cursor{MinUploadDate: time.Unix(42, 0), Page: 20},
				photos: nil,
			},
		},
		{name: "in bbox but not poly", region: denali,
			cursor: repos.Cursor{MinUploadDate: time.Unix(0, 0), Page: 1},
			fc: mockFC(t).expectCall(
				"flickr.photos.search",
				nil,
				[]byte(`{
				  "photos": {
					"photo": [
					  {
						"id": "34742333",
						"owner": "39916636@N00",
						"license": "5",
						"latitude": "63.4936134",
						"longitude": "-152.1032309",
						"dateupload": "1151810509"
				      }
					]
				 }
				}`),
				nil,
			),
			expect: Expect{
				cursor: repos.Cursor{MinUploadDate: time.Unix(0, 0), Page: 1},
				photos: nil,
			},
		},
	}

	for _, tt := range table {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			cursor, photos, err := Step(ctx, tt.fc, tt.mc, tt.region, tt.cursor)
			if err != nil {
				t.Fatal(err)
			}

			if cursor.Page != tt.expect.cursor.Page {
				t.Errorf("expected cursor.Page %v, got %v", tt.expect.cursor.Page, cursor.Page)
			}
			if cursor.MinUploadDate != tt.expect.cursor.MinUploadDate {
				t.Errorf("expected cursor.MinUploadDate %v, got %v", tt.expect.cursor.MinUploadDate, cursor.MinUploadDate)
			}

			if len(photos) != len(tt.expect.photos) {
				t.Errorf("expected %d photos, got %d", len(tt.expect.photos), len(photos))
			}
			for i, p := range photos {
				exp := tt.expect.photos[i]
				assert.Equal(t, exp.Id, p.Id)
				assert.Equal(t, exp.RegionId, p.RegionId)
				assert.JSONEq(t, string(exp.Info), string(p.Info))
				assert.JSONEq(t, string(exp.Sizes), string(p.Sizes))
				assert.JSONEq(t, string(exp.Exif), string(p.Exif))
			}
		})
	}
}

func mockFC(t *testing.T) *flickrMock {
	return &flickrMock{t: t}
}

type flickrMock struct {
	t        *testing.T
	expects  []fcExpect
	manyCall map[string]manyCall
	manyDl   *manyDl
}

type manyCall struct {
	response []byte
	err      error
}

type manyDl struct {
	body []byte
	err  error
}

type fcExpect struct {
	call *fcCall
	dl   *fcDl
}

type fcCall struct {
	method   string
	params   map[string]string
	response []byte
	err      error
}

type fcDl struct {
	url  string
	body []byte
	err  error
}

func (m *flickrMock) expectCall(method string, params map[string]string, response []byte, err error) *flickrMock {
	m.expects = append(m.expects, fcExpect{call: &fcCall{method, params, response, err}})
	return m
}

func (m *flickrMock) expectManyCall(method string, response []byte, err error) *flickrMock {
	if m.manyCall == nil {
		m.manyCall = make(map[string]manyCall)
	}
	m.manyCall[method] = manyCall{response, err}
	return m
}

func (m *flickrMock) expectDl(url string, body []byte, err error) *flickrMock {
	m.expects = append(m.expects, fcExpect{dl: &fcDl{url, body, err}})
	return m
}

func (m *flickrMock) expectManyDl(body []byte, err error) *flickrMock {
	m.manyDl = &manyDl{body, err}
	return m
}

func (m *flickrMock) Call(_ context.Context, method string, out any, params map[string]string) error {
	if exp, ok := m.manyCall[method]; ok {
		if exp.err != nil {
			return exp.err
		} else {
			return json.Unmarshal(exp.response, out)
		}
	}

	if len(m.expects) == 0 {
		m.t.Fatalf("unexpected call to Call")
	}
	expected := m.expects[0]
	m.expects = m.expects[1:]

	if expected.call != nil {
		exp := expected.call

		if exp.method != method {
			m.t.Errorf("expected method %q, got %q", exp.method, method)
		}

		if exp.params != nil {
			for k, gotV := range params {
				expectedV, ok := exp.params[k]
				if !ok {
					m.t.Errorf("unexpected param %q", k)
				}
				if gotV != expectedV {
					m.t.Errorf("expected param %q to be %q, got %q", k, expectedV, gotV)
				}
			}
			for k := range exp.params {
				if _, ok := params[k]; !ok {
					m.t.Errorf("expected param %q", k)
				}
			}
		}

		if exp.err != nil {
			return exp.err
		} else {
			return json.Unmarshal(exp.response, out)
		}
	} else {
		m.t.Fatalf("unexpected call to Call")
		return nil
	}
}

func (m *flickrMock) Download(_ context.Context, url string) (io.ReadCloser, error) {
	if m.manyDl != nil {
		return io.NopCloser(bytes.NewReader(m.manyDl.body)), nil
	}

	if len(m.expects) == 0 {
		m.t.Error("unexpected call to Download")
		panic("unexpected call to Download")
	}
	expected := m.expects[0]
	m.expects = m.expects[1:]

	if expected.dl != nil {
		exp := expected.dl
		if exp.url != url {
			m.t.Errorf("expected url %q, got %q", exp.url, url)
			panic("expected url")
		}
		if exp.err != nil {
			return nil, exp.err
		}
		return io.NopCloser(bytes.NewReader(exp.body)), nil
	} else {
		m.t.Error("unexpected call to Download")
		panic("unexpected call to Download")
		return nil, nil
	}
}

func mockMC(t *testing.T) *mcMock {
	return &mcMock{t: t}
}

type mcMock struct {
	t       *testing.T
	mu      sync.Mutex
	isNoop  bool
	expects []mcExpect
}

type mcExpect struct {
	fPutObject *mcExpectFPutObject
}

type mcExpectFPutObject struct {
	bucketName, objectName string
	opts                   minio.PutObjectOptions
	info                   minio.UploadInfo
	err                    error
}

func (m *mcMock) expectFPutObject(bucketName, objectName string, opts minio.PutObjectOptions, info minio.UploadInfo, err error) *mcMock {
	m.expects = append(m.expects, mcExpect{fPutObject: &mcExpectFPutObject{bucketName, objectName, opts, info, err}})
	return m
}

func (m *mcMock) noop() *mcMock {
	m.isNoop = true
	return m
}

func (m *mcMock) FPutObject(_ context.Context, bucketName, objectName string, _ string, opts minio.PutObjectOptions) (info minio.UploadInfo, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isNoop {
		return minio.UploadInfo{}, nil
	}

	if len(m.expects) == 0 {
		m.t.Fatalf("unexpected call to FPutObject")
	}
	expected := m.expects[0]
	m.expects = m.expects[1:]

	if expected.fPutObject != nil {
		exp := expected.fPutObject
		assert.Equal(m.t, exp.bucketName, bucketName)
		assert.Equal(m.t, exp.objectName, objectName)
		assert.Equal(m.t, exp.opts, opts)
		return exp.info, exp.err
	} else {
		m.t.Fatalf("unexpected call to FPutObject")
		return minio.UploadInfo{}, nil
	}
}

func makeRegion(v string) repos.Region {
	geo, err := wkt.UnmarshalMultiPolygon(v)
	if err != nil {
		panic(err)
	}
	return repos.Region{
		Id:  42,
		Geo: geo,
	}
}
