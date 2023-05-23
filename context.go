package grpcproxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	code_encoding "codeup.aliyun.com/qimao/leo/lib/code-encoding"
	"codeup.aliyun.com/qimao/leo/lib/code-encoding/form"

	"github.com/gin-gonic/gin"
	"github.com/go-leo/gox/stringx"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	metadataGrpcTimeout        = "Grpc-Timeout"
	xForwardedFor              = "X-Forwarded-For"
	xForwardedHost             = "X-Forwarded-Host"
	authorization              = "Authorization"
	gRPCMetadataHeaderPrefix   = "Grpc-Metadata-"
	gRPCMetadataTrailerPrefix  = "Grpc-Trailer-"
	metadataHeaderBinarySuffix = "-Bin"
)

func Bind(c *gin.Context, req proto.Message) error {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return err
	}
	return protojson.Unmarshal(body, req)
}

func GetBind(c *gin.Context, req proto.Message) error {
	return code_encoding.GetCodec(form.Name).Unmarshal([]byte(c.Request.URL.Query().Encode()), req)
}

func NewContext(c *gin.Context) context.Context {
	ctx, _ := contextWithTimeout(c)
	var pairs []string
	pairs = append(pairs, getAuthorization(c)...)
	pairs = append(pairs, getXForwardedHost(c)...)
	pairs = append(pairs, getXForwardedFor(c)...)
	pairs = append(pairs, getMetadataHeaderPrefix(c)...)
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...))
}

func Render(c *gin.Context, headerMD metadata.MD, trailerMD metadata.MD, resp proto.Message, err error) {
	st := status.Convert(err)
	if st != nil {
		data := []byte(fmt.Sprintf(`{"code":%d,"msg":"%s","data":null}`, st.Code(), st.Message()))
		c.Data(http.StatusOK, "application/json; charset=utf-8", data)
		return
	}
	writeHeader(c, headerMD)

	te := c.GetHeader("TE")
	doForwardTrailers := strings.Contains(strings.ToLower(te), "trailers")
	if doForwardTrailers {
		writeTrailerHeader(c, trailerMD)
	}

	respJson, err := protojson.Marshal(resp)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		_ = c.Error(err).SetType(gin.ErrorTypePrivate)
		return
	}
	data := []byte(fmt.Sprintf(`{"code":%d,"msg":"%s","data":%s}`, st.Code(), st.Message(), respJson))
	c.Data(http.StatusOK, "application/json; charset=utf-8", data)

	if doForwardTrailers {
		writeTrailer(c, trailerMD)
	}
}

func contextWithTimeout(c *gin.Context) (context.Context, context.CancelFunc) {
	tm := c.GetHeader(metadataGrpcTimeout)
	if stringx.IsNotBlank(tm) {
		timeout, err := decodeTimeout(tm)
		if err != nil {
			return c, nil
		}
		return context.WithTimeout(c, timeout)
	}
	return c, nil
}

func getAuthorization(c *gin.Context) []string {
	if auth := c.GetHeader(authorization); stringx.IsNotBlank(auth) {
		return []string{strings.ToLower(authorization), auth}
	}
	return nil
}

func getXForwardedHost(c *gin.Context) []string {
	if host := c.GetHeader(xForwardedHost); host != "" {
		return []string{strings.ToLower(xForwardedHost), host}
	} else if host = c.Request.Host; host != "" {
		return []string{strings.ToLower(xForwardedHost), host}
	}
	return nil
}

func getXForwardedFor(c *gin.Context) []string {
	clientIP := c.ClientIP()
	if stringx.IsBlank(clientIP) {
		return nil
	}
	fwd := c.GetHeader(xForwardedFor)
	if fwd == "" {
		return []string{strings.ToLower(xForwardedFor), clientIP}
	}
	return []string{strings.ToLower(xForwardedFor), fmt.Sprintf("%s, %s", fwd, clientIP)}
}

func getMetadataHeaderPrefix(c *gin.Context) []string {
	var pairs []string
	for key, vals := range c.Request.Header {
		key = textproto.CanonicalMIMEHeaderKey(key)
		for _, val := range vals {
			if strings.HasPrefix(key, gRPCMetadataHeaderPrefix) {
				if strings.HasSuffix(key, metadataHeaderBinarySuffix) {
					b, err := decodeBinHeader(val)
					if err != nil {
						continue
					}
					val = string(b)
				}
				pairs = append(pairs, strings.ToLower(key), val)
			}
		}
	}
	return pairs
}

func decodeTimeout(s string) (time.Duration, error) {
	size := len(s)
	if size < 2 {
		return 0, fmt.Errorf("transport: timeout string is too short: %q", s)
	}
	if size > 9 {
		// Spec allows for 8 digits plus the unit.
		return 0, fmt.Errorf("transport: timeout string is too long: %q", s)
	}
	unit := timeoutUnit(s[size-1])
	d, ok := timeoutUnitToDuration(unit)
	if !ok {
		return 0, fmt.Errorf("transport: timeout unit is not recognized: %q", s)
	}
	t, err := strconv.ParseInt(s[:size-1], 10, 64)
	if err != nil {
		return 0, err
	}
	const maxHours = math.MaxInt64 / int64(time.Hour)
	if d == time.Hour && t > maxHours {
		// This timeout would overflow math.MaxInt64; clamp it.
		return time.Duration(math.MaxInt64), nil
	}
	return d * time.Duration(t), nil
}

type timeoutUnit uint8

const (
	hour        timeoutUnit = 'H'
	minute      timeoutUnit = 'M'
	second      timeoutUnit = 'S'
	millisecond timeoutUnit = 'm'
	microsecond timeoutUnit = 'u'
	nanosecond  timeoutUnit = 'n'
)

func timeoutUnitToDuration(u timeoutUnit) (d time.Duration, ok bool) {
	switch u {
	case hour:
		return time.Hour, true
	case minute:
		return time.Minute, true
	case second:
		return time.Second, true
	case millisecond:
		return time.Millisecond, true
	case microsecond:
		return time.Microsecond, true
	case nanosecond:
		return time.Nanosecond, true
	default:
	}
	return
}

func decodeBinHeader(v string) ([]byte, error) {
	if len(v)%4 == 0 {
		// Input was padded, or padding was not necessary.
		return base64.StdEncoding.DecodeString(v)
	}
	return base64.RawStdEncoding.DecodeString(v)
}

func writeHeader(c *gin.Context, headerMD metadata.MD) {
	for key, vals := range headerMD {
		for _, val := range vals {
			c.Writer.Header().Add(gRPCMetadataHeaderPrefix+key, val)
		}
	}
}

func writeTrailerHeader(c *gin.Context, trailerMD metadata.MD) {
	for key := range trailerMD {
		c.Writer.Header().Add("Trailer", gRPCMetadataTrailerPrefix+key)
	}
	c.Header("Transfer-Encoding", "chunked")
}

func writeTrailer(c *gin.Context, trailerMD metadata.MD) {
	for key, vals := range trailerMD {
		tKey := gRPCMetadataTrailerPrefix + key
		for _, v := range vals {
			c.Writer.Header().Add(tKey, v)
		}
	}
}
