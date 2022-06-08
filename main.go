package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/honeycombio/libhoney-go"
	"github.com/klauspost/compress/zstd"
	collectorLog "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/proto"
)

const (
	traceIDShortLength = 8
	traceIDLongLength  = 16
)

func main() {

	libhoney.UserAgentAddition = "http-honeyotellog"
	// Initialize and configure libhoney
	err := libhoney.Init(libhoney.Config{
		APIKey:  os.Getenv("HONEYCOMB_API_KEY"),
		Dataset: os.Getenv("HONEYCOMB_DATASET"),
	})
	if err != nil {
		fmt.Printf("fatal error initializing libhoney: %v\n", err)
		os.Exit(2)
	}
	libhoney.AddField("event.parser", "http-honeyotellog")
	defer libhoney.Close() // Flush any pending calls to Honeycomb

	// Create HTTP server and primary handler
	server := &http.Server{Addr: ":8080"}
	http.HandleFunc("/v1/logs", readNewData)
	go func() {
		if err := server.ListenAndServe(); err != nil {
			// handle err
		}
	}()

	// set up signal capturing
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	// Waiting for SIGINT (kill -2)
	<-stop
	libhoney.Flush()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		// handle err
	}
}

type OTLPError struct {
	Message        string
	HTTPStatusCode int
}

func addAttributesToMap(attrs map[string]interface{}, attributes []*common.KeyValue) {
	for _, attr := range attributes {
		// ignore entries if the key is empty or value is nil
		if attr.Key == "" || attr.Value == nil {
			continue
		}
		if val := getValue(attr.Value); val != nil {
			if strings.Index(attr.Key, "Scope") >= 0 {
				betterkey := attr.Key[strings.Index(attr.Key, ":")+1 : len(attr.Key)]
				attrs[betterkey] = val
			} else {

				attrs[attr.Key] = val
			}

		}
	}
}
func getValue(value *common.AnyValue) interface{} {
	switch value.Value.(type) {
	case *common.AnyValue_StringValue:
		return value.GetStringValue()
	case *common.AnyValue_BoolValue:
		return value.GetBoolValue()
	case *common.AnyValue_DoubleValue:
		return value.GetDoubleValue()
	case *common.AnyValue_IntValue:
		return value.GetIntValue()
	case *common.AnyValue_ArrayValue:
		items := value.GetArrayValue().Values
		arr := make([]interface{}, len(items))
		for i := 0; i < len(items); i++ {
			arr[i] = getValue(items[i])
		}
		bytes, err := json.Marshal(arr)
		if err == nil {
			return string(bytes)
		}
	case *common.AnyValue_KvlistValue:
		items := value.GetKvlistValue().Values
		arr := make([]map[string]interface{}, len(items))
		for i := 0; i < len(items); i++ {
			arr[i] = map[string]interface{}{
				items[i].Key: getValue(items[i].Value),
			}
		}
		bytes, err := json.Marshal(arr)
		if err == nil {
			return string(bytes)
		}
	}
	return nil
}

func readNewData(w http.ResponseWriter, r *http.Request) {

	request, err := parseOTLPBody(r.Body, r.Header.Get("Content-Encoding"))
	if err != nil {
		fmt.Printf("failed to parse OTLP request body: %v\n", err)
		return
	}
	for _, resourceLog := range request.ResourceLogs {
		resourceAttrs := make(map[string]interface{})
		if resourceLog.Resource != nil {

			addAttributesToMap(resourceAttrs, resourceLog.Resource.Attributes)
		}

		for _, scopeLog := range resourceLog.ScopeLogs {

			for _, logRecord := range scopeLog.LogRecords {
				ev := libhoney.NewEvent()
				ev.Add(resourceAttrs)
				logAttrs := make(map[string]interface{})
				if logRecord.Attributes != nil {
					addAttributesToMap(logAttrs, logRecord.Attributes)
				}
				ev.Add(resourceAttrs)
				ev.Add(logAttrs)
				ev.AddField("name", getValue(logRecord.Body))
				traceID := BytesToTraceID(logRecord.TraceId)
				spanID := hex.EncodeToString(logRecord.SpanId)
				timestamp := time.Unix(0, int64(logRecord.TimeUnixNano)).UTC()
				ev.Timestamp = timestamp
				ev.AddField("SeverityText", logRecord.SeverityText)
				ev.AddField("SeverityNumber", logRecord.SeverityNumber)

				ev.AddField("trace.trace_id", traceID)
				ev.AddField("trace.parent_id", spanID)
				ev.AddField("meta.annotation_type", "span_event")
				err = ev.Send()
				if err != nil {
					fmt.Printf("event send error %v\n", err)
					continue
				}
			}

		}
	}

	w.WriteHeader(200)
}
func BytesToTraceID(traceID []byte) string {
	var encoded []byte
	switch len(traceID) {
	case traceIDLongLength: // 16 bytes, trim leading 8 bytes if all 0's
		if shouldTrimTraceId(traceID) {
			encoded = make([]byte, 16)
			traceID = traceID[traceIDShortLength:]
		} else {
			encoded = make([]byte, 32)
		}
	case traceIDShortLength: // 8 bytes
		encoded = make([]byte, 16)
	default:
		encoded = make([]byte, len(traceID)*2)
	}
	hex.Encode(encoded, traceID)
	return string(encoded)
}
func shouldTrimTraceId(traceID []byte) bool {
	for i := 0; i < 8; i++ {
		if traceID[i] != 0 {
			return false
		}
	}
	return true
}
func parseOTLPBody(body io.ReadCloser, contentEncoding string) (request *collectorLog.ExportLogsServiceRequest, err error) {
	defer body.Close()
	bodyBytes, err := ioutil.ReadAll(body)
	if err != nil {
		fmt.Sprintf("BodyBytes: %v", err)
		return nil, err
	}

	bodyReader := bytes.NewReader(bodyBytes)

	var reader io.Reader
	switch contentEncoding {
	case "gzip":
		gzipReader, err := gzip.NewReader(bodyReader)
		defer gzipReader.Close()
		if err != nil {
			return nil, err
		}
		reader = gzipReader
	case "zstd":
		zstdReader, err := zstd.NewReader(bodyReader)
		defer zstdReader.Close()
		if err != nil {
			return nil, err
		}
		reader = zstdReader
	default:
		reader = bodyReader
	}
	request = &collectorLog.ExportLogsServiceRequest{}
	data, err := ioutil.ReadAll(reader)
	err = proto.Unmarshal(data, request)
	if err != nil {
		return nil, err
	}

	return request, nil
}
