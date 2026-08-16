// Boots the API reference on api.html.
//
// This lives in a file rather than in a <script> block so that the page's
// Content-Security-Policy can name the exact origins allowed to supply script
// and refuse everything else. An inline block would force 'unsafe-inline',
// which switches off most of what the policy is for.
Scalar.createApiReference('#reference', {
  url: './openapi.yaml',
  theme: 'default',
  layout: 'modern',
  hideDownloadButton: false,
  metaData: { title: 'tgju-api-go — API reference' },
})
