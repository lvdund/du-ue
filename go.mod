module du_ue

go 1.24.4

require (
	github.com/JocelynWS/f1-gen v1.0.6
	github.com/ishidawataru/sctp v0.0.0-20251114114122-19ddcbc6aae2
	github.com/lvdund/asn1go v1.0.6
	github.com/lvdund/ngap v1.4.13
	github.com/lvdund/rrc v1.0.6
	github.com/reogac/nas v1.1.13
	github.com/reogac/utils v1.1.15
	github.com/rs/zerolog v1.34.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/aead/cmac v0.0.0-20160719120800-7af84192f0b1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.12.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	golang.org/x/sys v0.33.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

replace github.com/free5gc/webconsole => github.com/lvdund/webconsole v0.0.0-20250905084622-37e7ec5ad5d2

replace github.com/reogac/etrib5gc => ./third_party/etrib5gc

replace github.com/JocelynWS/f1-gen => ./third_party/f1-gen
