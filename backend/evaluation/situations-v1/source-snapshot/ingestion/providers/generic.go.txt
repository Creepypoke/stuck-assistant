package providers

import (
	"context"
	"net/url"
	"strings"

	"github.com/walkthrough-assistance/backend/ingestion"
)

// GenericURL delegates a manually supplied URL to a document downloader.
type GenericURL struct {
	downloader ingestion.WalkthroughProvider
}

func NewGenericURL(downloader ingestion.WalkthroughProvider) *GenericURL {
	return &GenericURL{downloader: downloader}
}

func (p *GenericURL) Download(ctx context.Context, source ingestion.Source) (ingestion.RawDocument, error) {
	return p.downloader.Download(ctx, source)
}

// URLOrFile delegates HTTP(S) sources to a downloader and local paths to a file reader.
type URLOrFile struct {
	url  ingestion.WalkthroughProvider
	file ingestion.WalkthroughProvider
}

func NewURLOrFile(urlProvider, fileProvider ingestion.WalkthroughProvider) *URLOrFile {
	return &URLOrFile{url: urlProvider, file: fileProvider}
}

func (p *URLOrFile) Download(ctx context.Context, source ingestion.Source) (ingestion.RawDocument, error) {
	if isHTTPURL(source.URL) {
		return p.url.Download(ctx, source)
	}
	return p.file.Download(ctx, source)
}

func isHTTPURL(source string) bool {
	parsed, err := url.Parse(source)
	return err == nil && parsed.Host != "" && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https"))
}
