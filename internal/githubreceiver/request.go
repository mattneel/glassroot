package githubreceiver

import (
	"errors"
	"net/http"

	"github.com/mattneel/glassroot/internal/githubapp"
)

func parseRequestHeaders(r *http.Request, limits Limits) (githubapp.WebhookHeaders, error) {
	var values []githubapp.HeaderValue
	for _, name := range []string{"X-GitHub-Delivery", "X-GitHub-Event", "X-Hub-Signature-256", "Content-Type", "Content-Encoding", "X-GitHub-Hook-ID", "X-GitHub-Hook-Installation-Target-ID", "X-GitHub-Hook-Installation-Target-Type"} {
		for _, v := range r.Header.Values(name) {
			values = append(values, githubapp.HeaderValue{Name: name, Value: v})
		}
	}
	return githubapp.ParseWebhookHeaders(values, limits.GitHub)
}

func statusForHeaderError(err error) int {
	if errors.Is(err, githubapp.ErrCode(githubapp.CodeMissingSignature)) || errors.Is(err, githubapp.ErrCode(githubapp.CodeDuplicateSignatureHeader)) {
		return http.StatusUnauthorized
	}
	if errors.Is(err, githubapp.ErrCode(githubapp.CodeInvalidContentType)) || errors.Is(err, githubapp.ErrCode(githubapp.CodeUnsupportedContentEncoding)) {
		return http.StatusUnsupportedMediaType
	}
	return http.StatusBadRequest
}

func codeForError(err error) ErrorCode {
	if errors.Is(err, githubapp.ErrCode(githubapp.CodeInvalidContentType)) {
		return CodeInvalidContentType
	}
	if errors.Is(err, githubapp.ErrCode(githubapp.CodeUnsupportedContentEncoding)) {
		return CodeUnsupportedContentEncoding
	}
	if errors.Is(err, githubapp.ErrCode(githubapp.CodeMissingSignature)) || errors.Is(err, githubapp.ErrCode(githubapp.CodeDuplicateSignatureHeader)) {
		return CodeSignatureInvalid
	}
	return CodeProjectionInvalid
}
