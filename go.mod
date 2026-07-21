module github.com/scp-stk/hub

go 1.25.0

require (
	github.com/go-chi/chi/v5 v5.0.12
	github.com/go-chi/cors v1.2.1
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.6.0
	github.com/joho/godotenv v1.5.1
)

require (
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/hhrutter/lzw v1.0.0 // indirect
	github.com/hhrutter/pkcs7 v0.2.2 // indirect
	github.com/hhrutter/tiff v1.0.3 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/mattn/go-runewidth v0.0.24 // indirect
	github.com/pdfcpu/pdfcpu v0.13.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/image v0.41.0 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

// These replace lines exist only because some build environments restrict
// network access to proxy.golang.org/golang.org/gopkg.in. They pin the exact
// same versions via GitHub mirrors and are harmless to leave in — Railway,
// Fly.io, and any normal CI will build fine with or without them.
replace gopkg.in/yaml.v3 => github.com/go-yaml/yaml v0.0.0-20220512140231-539c8e751b99

replace gopkg.in/check.v1 => github.com/go-check/check v0.0.0-20180628173108-788fd7840127

replace golang.org/x/crypto => github.com/golang/crypto v0.17.0

replace golang.org/x/text => github.com/golang/text v0.14.0

replace golang.org/x/sync => github.com/golang/sync v0.1.0
