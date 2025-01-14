// The MIT License (MIT)

// Copyright (c) John Bryan Sazon <bryansazon@hotmail.com>

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package s3

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	htmltemp "html/template"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/bzon/gota/parser"
)

// Upload is used by UploadFile
type Upload struct {
	Bucket, SrcFile, DestFilePath string
}

// UploadFile accepts a struct of type Upload.
// It is assumed that the caller has AWS_ACCESS_KEY and AWS_ACCESS_SECRET_KEY is defined as env variable.
//
// API doc: https://github.com/aws/aws-sdk-go/blob/master/service/s3/s3manager/upload.go#L218-L252
func UploadFile(upload Upload) (string, error) {
	f, err := os.Open(upload.SrcFile)
	if err != nil {
		return "", err
	}
	defer f.Close()

	fileInfo, _ := f.Stat()
	var size int64 = fileInfo.Size()
	buffer := make([]byte, size)
	_, err = f.Read(buffer)
	if err != nil {
		return "", err
	}
	contentType := http.DetectContentType(buffer)
	// Set the content-type so that the file like index.html can be viewed directly from the browser
	if strings.Contains(upload.SrcFile, "html") {
		contentType = "text/html"
	}
	if strings.Contains(upload.SrcFile, "apk") {
		contentType = "application/vnd.android.package-archive"
	}

	// Create a new session
	sess := session.Must(session.NewSession())

	// Create an uploader with session and default options
	uploader := s3manager.NewUploader(sess)

	// Upload to S3 bucket
	result, err := uploader.Upload(&s3manager.UploadInput{
		ACL:    aws.String("public-read"),
		Bucket: aws.String(upload.Bucket),
		Key:    aws.String(upload.DestFilePath),
		// At this stage, the file that will be uploaded becomes an empty file because f.Read(buffer) already moved f to the last byte.
		// So we use bytes.NewReader to read buffer from the first byte instead of passing in f
		// If we do not do this, the uploaded file will have 0 byte which is equal to an empty file.
		Body:                 bytes.NewReader(buffer),
		ContentType:          aws.String(contentType),
		ServerSideEncryption: aws.String("AES256"),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload file, %v", err)
	}
	return aws.StringValue(&result.Location), nil
}

// UploadAssets uploads the generated files by the parser package along with the ipa or apk file.
// It accepts a struct of type *parser.MobileApp and the destination base directory of the s3 bucket.
// It returns a slice of aws s3 files url.
func UploadAssets(app *parser.MobileApp, bucket, destBaseDir string) ([]string, error) {
	// create the site path names and assume the url before uploaded for templating
	buildDir := destBaseDir + "/" + app.Version + "/" + app.Build
	appIconPath := buildDir + "/" + parser.AppIconFile
	appSitePath := buildDir + "/" + filepath.Base(app.File)
	appIndexHTMLSitePath := buildDir + "/" + parser.IndexHTMLFile
	bucketHTTPSurl := "https://" + bucket + ".s3.amazonaws.com"
	app.DownloadURL = bucketHTTPSurl + "/" + appSitePath

	// default directory of assets
	assetsDir := parser.AndroidAssetsDir
	// specific for ios
	var appPlistSitePath string
	if app.IsIOS() {
		assetsDir = parser.IOSAssetsDir
		appPlistSitePath = buildDir + "/" + parser.IOSPlistFile
		app.PlistURL = htmltemp.URL(bucketHTTPSurl + "/" + appPlistSitePath)
	}

	// create the assets
	assets := []string{}
	if err := app.GenerateAssets(); err != nil {
		return assets, err
	}

	uploads := []Upload{
		{bucket, assetsDir + "/" + parser.AppIconFile, appIconPath},
		{bucket, assetsDir + "/" + parser.VersionJsonFile, destBaseDir + "/" + app.Version + "/" + parser.VersionJsonFile},
		{bucket, assetsDir + "/" + parser.IndexHTMLFile, appIndexHTMLSitePath},
		{bucket, app.File, appSitePath},
	}

	if app.IsIOS() {
		uploads = append(uploads, Upload{bucket, assetsDir + "/" + parser.IOSPlistFile, appPlistSitePath})
	}

	for _, upload := range uploads {
		fileURL, err := UploadFile(upload)
		if err != nil {
			return assets, err
		}
		// Ensure the returned string is a decoded url
		decodedURL, err := url.QueryUnescape(fileURL)
		if err != nil {
			return assets, err
		}
		assets = append(assets, decodedURL)
	}

	return assets, nil
}
