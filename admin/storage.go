package admin

import (
	"context"
	"encoding/json"
	"github.com/minio/minio-go/v7"
	"net/http"
	"slices"
	"sync"
	"time"
)

const photosBucket = "contourguessr-photos"

func storageHandler(w http.ResponseWriter, r *http.Request) {
	data, err := fetchStorageData(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	histogramBucketsJSON, err := json.Marshal(data.HistogramBuckets)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	runningSizeJSON, err := json.Marshal(data.RunningSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	templateResponse(w, r, "storage.tmpl.html", M{
		"D":                    data,
		"HistogramBucketsJSON": string(histogramBucketsJSON),
		"RunningSizeJSON":      string(runningSizeJSON),
	})
}

type storageData struct {
	TotalCount        int
	TotalSize         int64
	SizeAllGreater4MB int64
	MeanSize          int64
	MedianSize        int64
	HistogramBuckets  []storageHistogramBucket
	RunningSize       []runningSizeEntry
}

type storageHistogramBucket struct {
	Size  int64
	Count int
}

type runningSizeEntry struct {
	Time  time.Time
	Size  int64
	Count int
}

func fetchStorageData(ctx context.Context) (*storageData, error) {
	data := &storageData{}

	objects, err := fetchObjectInfo(ctx)
	if err != nil {
		return nil, err
	}

	var earliest time.Time
	buckets := make(map[int64]int)
	for _, obj := range objects {
		size := obj.Size

		bucket := size / 1024 / 1024
		buckets[bucket]++

		data.TotalCount++
		data.TotalSize += size
		if size > 4*1024*1024 {
			data.SizeAllGreater4MB += size
		}

		if earliest.IsZero() || obj.LastModified.Before(earliest) {
			earliest = obj.LastModified
		}
	}
	data.MeanSize = data.TotalSize / int64(data.TotalCount)

	if len(objects) > 0 {
		data.MedianSize = objects[len(objects)/2].Size
	}

	var bucketList []storageHistogramBucket
	for size, count := range buckets {
		bucketList = append(bucketList, storageHistogramBucket{Size: size, Count: count})
	}
	data.HistogramBuckets = bucketList

	for cutoff := earliest.Add(time.Hour); cutoff.Before(time.Now()); cutoff = cutoff.Add(time.Hour) {
		entry := runningSizeEntry{Time: cutoff}
		for _, obj := range objects {
			if obj.LastModified.Before(cutoff) {
				entry.Size += obj.Size
				entry.Count++
			}
		}
		data.RunningSize = append(data.RunningSize, entry)
	}

	return data, nil
}

type objectInfo struct {
	Name         string
	Size         int64
	LastModified time.Time
}

var objectInfoCacheMu sync.Mutex
var objectInfoCache []objectInfo
var objectInfoCacheUpdated time.Time

func fetchObjectInfo(ctx context.Context) ([]objectInfo, error) {
	objectInfoCacheMu.Lock()
	defer objectInfoCacheMu.Unlock()
	if time.Since(objectInfoCacheUpdated) < 1*time.Hour {
		return objectInfoCache, nil
	}

	var out []objectInfo
	for obj := range mc.ListObjects(ctx, photosBucket, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		out = append(out, objectInfo{
			Name:         obj.Key,
			Size:         obj.Size,
			LastModified: obj.LastModified,
		})
	}
	slices.SortFunc(out, func(a, b objectInfo) int {
		return int(a.Size - b.Size)
	})

	objectInfoCache = out
	objectInfoCacheUpdated = time.Now()

	return out, nil
}
