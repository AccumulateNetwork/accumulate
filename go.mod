module gitlab.com/accumulatenetwork/accumulate

retract (
	v1.1.1 // Tagged the wrong commit
	v1.1.0 // Causes consensus failure when run alongside v1.0.x
)

go 1.22.1

replace github.com/julienschmidt/httprouter v1.3.0 => github.com/firelizzard18/httprouter v0.0.0-20231019203155-74063b4f447c

replace golang.org/x/tools v0.19.0 => golang.org/x/tools v0.18.0

require (
	github.com/AccumulateNetwork/jsonrpc2/v15 v15.0.0-20220517212445-953ad957e040
	github.com/FactomProject/factom v0.4.0
	github.com/btcsuite/btcutil v1.0.3-0.20201208143702-a53e38424cce
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	github.com/dgraph-io/badger v1.6.2
	github.com/dustin/go-humanize v1.0.1
	github.com/fatih/color v1.16.0
	github.com/go-playground/validator/v10 v10.9.0
	github.com/golangci/golangci-lint v1.56.3-0.20240311192827-d18acc5b51ed
	github.com/pelletier/go-toml v1.9.5
	github.com/prometheus/client_golang v1.19.1
	github.com/prometheus/common v0.53.0
	github.com/rinchsan/gosimports v0.1.5
	github.com/rs/zerolog v1.29.0
	github.com/spf13/cobra v1.7.0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.15.0
	github.com/stretchr/testify v1.9.0
	gitlab.com/ethan.reesor/vscode-notebooks/go-playbooks v0.0.0-20220417214602-1121b9fae118
	gitlab.com/ethan.reesor/vscode-notebooks/yaegi v0.0.0-20220417214422-5c573557938e
	golang.org/x/exp v0.0.0-20240416160154-fe59bbe5cc7f
	golang.org/x/sync v0.7.0
	golang.org/x/sys v0.20.0
	golang.org/x/term v0.20.0
	golang.org/x/text v0.15.0
	golang.org/x/tools v0.20.0
	gopkg.in/yaml.v3 v3.0.1
	gotest.tools/gotestsum v1.8.1
)

require (
	github.com/FactomProject/factomd v1.13.0
	github.com/chzyer/readline v1.5.1
	github.com/cometbft/cometbft v0.38.0-rc3
	github.com/cosmos/gogoproto v1.4.11
	github.com/dgraph-io/badger/v3 v3.2103.5
	github.com/dgraph-io/badger/v4 v4.2.0
	github.com/edsrzf/mmap-go v1.1.0
	github.com/ethereum/go-ethereum v1.10.25
	github.com/fatih/astrewrite v0.0.0-20191207154002-9094e544fcef
	github.com/gobeam/stringy v0.0.6
	github.com/golangci/plugin-module-register v0.0.0-20240305222101-f76272ec86ee
	github.com/google/pprof v0.0.0-20240207164012-fb44976bdcd5
	github.com/google/uuid v1.6.0
	github.com/ipfs/go-cid v0.4.1
	github.com/julienschmidt/httprouter v1.3.0
	github.com/libp2p/go-libp2p v0.33.1
	github.com/libp2p/go-libp2p-kad-dht v0.25.2
	github.com/libp2p/go-libp2p-pubsub v0.10.0
	github.com/mattn/go-shellwords v1.0.12
	github.com/mr-tron/base58 v1.2.0
	github.com/multiformats/go-multiaddr v0.12.2
	github.com/multiformats/go-multiaddr-fmt v0.1.0
	github.com/multiformats/go-multibase v0.2.0
	github.com/multiformats/go-multicodec v0.9.0
	github.com/multiformats/go-multihash v0.2.3
	github.com/robfig/cron/v3 v3.0.0
	github.com/sergi/go-diff v1.2.0
	github.com/ulikunitz/xz v0.5.11
	github.com/vektra/mockery/v2 v2.42.3
	gitlab.com/accumulatenetwork/core/schema v0.2.1-0.20240711192735-5b3657ff1135
	gitlab.com/firelizzard/go-script v0.0.0-20240404234115-d5f0a716003d
	go.opentelemetry.io/otel v1.27.0
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.27.0
	go.opentelemetry.io/otel/exporters/prometheus v0.49.0
	go.opentelemetry.io/otel/exporters/stdout/stdoutlog v0.3.0
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.27.0
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.14.0
	go.opentelemetry.io/otel/log v0.3.0
	go.opentelemetry.io/otel/metric v1.27.0
	go.opentelemetry.io/otel/sdk v1.27.0
	go.opentelemetry.io/otel/sdk/log v0.3.0
	go.opentelemetry.io/otel/sdk/metric v1.27.0
	go.opentelemetry.io/otel/trace v1.27.0
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.3.0
)

require (
	4d63.com/gocheckcompilerdirectives v1.2.1 // indirect
	github.com/4meepo/tagalign v1.3.3 // indirect
	github.com/Abirdcfly/dupword v0.0.14 // indirect
	github.com/Antonboom/testifylint v1.2.0 // indirect
	github.com/FactomProject/btcutil v0.0.0-20200312214114-5fd3eaf71bd2 // indirect
	github.com/FactomProject/ed25519 v0.0.0-20150814230546-38002c4fe7b6 // indirect
	github.com/FactomProject/go-bip32 v0.3.6-0.20161206200006-3b593af1c415 // indirect
	github.com/FactomProject/go-bip39 v0.3.6-0.20161217174232-d1007fb78d9a // indirect
	github.com/FactomProject/go-bip44 v0.0.0-20190306062959-b541a96d8da9 // indirect
	github.com/FactomProject/go-simplejson v0.5.0 // indirect
	github.com/FactomProject/goleveldb v0.2.2-0.20170418171130-e7800c6976c5 // indirect
	github.com/FactomProject/netki-go-partner-client v0.0.0-20160324224126-426acb535e66 // indirect
	github.com/GaijinEntertainment/go-exhaustruct/v3 v3.2.0 // indirect
	github.com/OneOfOne/xxhash v1.2.6 // indirect
	github.com/OpenPeeDeeP/depguard/v2 v2.2.0 // indirect
	github.com/alecthomas/go-check-sumtype v0.1.4 // indirect
	github.com/alexkohler/nakedret/v2 v2.0.2 // indirect
	github.com/alingse/asasalint v0.0.11 // indirect
	github.com/benbjohnson/clock v1.3.5 // indirect
	github.com/bombsimon/wsl/v4 v4.2.1 // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.3.2 // indirect
	github.com/btcsuite/btcd/chaincfg/chainhash v1.0.1 // indirect
	github.com/btcsuitereleases/btcutil v0.0.0-20150612230727-f2b1058a8255 // indirect
	github.com/butuzov/mirror v1.1.0 // indirect
	github.com/catenacyber/perfsprint v0.7.1 // indirect
	github.com/ccojocar/zxcvbn-go v1.0.2 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/chigopher/pathlib v0.19.1 // indirect
	github.com/ckaznocha/intrange v0.1.0 // indirect
	github.com/cometbft/cometbft-db v0.7.0 // indirect
	github.com/containerd/cgroups v1.1.0 // indirect
	github.com/curioswitch/go-reassign v0.2.0 // indirect
	github.com/davidlazar/go-crypto v0.0.0-20200604182044-b73af7476f6c // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/elastic/gosigar v0.14.2 // indirect
	github.com/firefart/nonamedreturns v1.0.4 // indirect
	github.com/flynn/noise v1.1.0 // indirect
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/ghostiam/protogetter v0.3.4 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/go-viper/mapstructure/v2 v2.0.0-alpha.1 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/golang/glog v1.2.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/flatbuffers v1.12.1 // indirect
	github.com/google/gopacket v1.1.19 // indirect
	github.com/gopherjs/gopherjs v0.0.0-20191106031601-ce3c9ade29de // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.20.0 // indirect
	github.com/hashicorp/golang-lru v0.5.5-0.20210104140557-80c98217689d // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.5 // indirect
	github.com/huandu/xstrings v1.4.0 // indirect
	github.com/huin/goupnp v1.3.0 // indirect
	github.com/iancoleman/strcase v0.2.0 // indirect
	github.com/ipfs/boxo v0.10.0 // indirect
	github.com/ipfs/go-datastore v0.6.0 // indirect
	github.com/ipfs/go-log v1.0.5 // indirect
	github.com/ipfs/go-log/v2 v2.5.1 // indirect
	github.com/ipld/go-ipld-prime v0.20.0 // indirect
	github.com/jackpal/go-nat-pmp v1.0.2 // indirect
	github.com/jbenet/go-temp-err-catcher v0.1.0 // indirect
	github.com/jbenet/goprocess v0.1.4 // indirect
	github.com/jinzhu/copier v0.3.5 // indirect
	github.com/jjti/go-spancheck v0.5.3 // indirect
	github.com/karamaru-alpha/copyloopvar v1.0.8 // indirect
	github.com/kkHAIKE/contextcheck v1.1.4 // indirect
	github.com/klauspost/compress v1.17.6 // indirect
	github.com/klauspost/cpuid/v2 v2.2.7 // indirect
	github.com/koron/go-ssdp v0.0.4 // indirect
	github.com/libp2p/go-cidranger v1.1.0 // indirect
	github.com/libp2p/go-flow-metrics v0.1.0 // indirect
	github.com/libp2p/go-libp2p-asn-util v0.4.1 // indirect
	github.com/libp2p/go-libp2p-kbucket v0.6.3 // indirect
	github.com/libp2p/go-libp2p-record v0.2.0 // indirect
	github.com/libp2p/go-libp2p-routing-helpers v0.7.2 // indirect
	github.com/libp2p/go-msgio v0.3.0 // indirect
	github.com/libp2p/go-nat v0.2.0 // indirect
	github.com/libp2p/go-netroute v0.2.1 // indirect
	github.com/libp2p/go-reuseport v0.4.0 // indirect
	github.com/libp2p/go-yamux/v4 v4.0.1 // indirect
	github.com/lufeee/execinquery v1.2.1 // indirect
	github.com/macabu/inamedparam v0.1.3 // indirect
	github.com/maratori/testableexamples v1.0.0 // indirect
	github.com/marten-seemann/tcp v0.0.0-20210406111302-dfbc87cc63fd // indirect
	github.com/miekg/dns v1.1.58 // indirect
	github.com/mikioh/tcpinfo v0.0.0-20190314235526-30a79bb1804b // indirect
	github.com/mikioh/tcpopt v0.0.0-20190314235656-172688c1accc // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mitchellh/go-testing-interface v1.0.0 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multiaddr-dns v0.3.1 // indirect
	github.com/multiformats/go-multistream v0.5.0 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/nunnatsa/ginkgolinter v0.15.2 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/oasisprotocol/curve25519-voi v0.0.0-20220708102147-0a8a51822cae // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/onsi/ginkgo/v2 v2.15.0 // indirect
	github.com/opencontainers/runtime-spec v1.2.0 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58 // indirect
	github.com/pelletier/go-toml/v2 v2.0.6 // indirect
	github.com/polydawn/refmt v0.89.0 // indirect
	github.com/quasilyte/stdinfo v0.0.0-20220114132959-f7386bf02567 // indirect
	github.com/quic-go/qpack v0.4.0 // indirect
	github.com/quic-go/quic-go v0.42.0 // indirect
	github.com/quic-go/webtransport-go v0.6.0 // indirect
	github.com/raulk/go-watchdog v1.3.0 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/sashamelentyev/interfacebloat v1.1.0 // indirect
	github.com/sashamelentyev/usestdlibvars v1.25.0 // indirect
	github.com/smartystreets/assertions v1.13.0 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/stbenjam/no-sprintf-host-port v0.1.1 // indirect
	github.com/t-yuki/gocover-cobertura v0.0.0-20180217150009-aaee18c8195c // indirect
	github.com/timonwong/loggercheck v0.9.4 // indirect
	github.com/whyrusleeping/go-keyspace v0.0.0-20160322163242-5b898ac5add1 // indirect
	github.com/xen0n/gosmopolitan v1.2.2 // indirect
	github.com/ykadowak/zerologlint v0.1.5 // indirect
	go-simpler.org/musttag v0.9.0 // indirect
	go-simpler.org/sloglint v0.4.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/proto/otlp v1.2.0 // indirect
	go.uber.org/automaxprocs v1.5.3 // indirect
	go.uber.org/dig v1.17.1 // indirect
	go.uber.org/fx v1.20.1 // indirect
	go.uber.org/mock v0.4.0 // indirect
	golang.org/x/exp/typeparams v0.0.0-20240213143201-ec583247a57a // indirect
	gonum.org/v1/gonum v0.13.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240520151616-dc85e6b867a5 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240515191416-fc5f0ca64291 // indirect
	gotest.tools/v3 v3.2.0 // indirect
	lukechampine.com/blake3 v1.2.1 // indirect
)

require (
	4d63.com/gochecknoglobals v0.2.1 // indirect
	github.com/AndreasBriese/bbloom v0.0.0-20190825152654-46b345b51c96 // indirect
	github.com/Antonboom/errname v0.1.12 // indirect
	github.com/Antonboom/nilnil v0.1.7 // indirect
	github.com/BurntSushi/toml v1.3.2
	github.com/Djarvur/go-err113 v0.0.0-20210108212216-aea10b59be24 // indirect
	github.com/FactomProject/basen v0.0.0-20150613233007-fe3947df716e // indirect
	github.com/FactomProject/btcutilecc v0.0.0-20130527213604-d3a63a5752ec // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/alexkohler/prealloc v1.0.0 // indirect
	github.com/ashanbrown/forbidigo v1.6.0 // indirect
	github.com/ashanbrown/makezero v1.1.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bkielbasa/cyclop v1.2.1 // indirect
	github.com/blizzy78/varnamelen v0.8.0 // indirect
	github.com/breml/bidichk v0.2.7 // indirect
	github.com/breml/errchkjson v0.3.6 // indirect
	github.com/btcsuite/btcd v0.22.1
	github.com/butuzov/ireturn v0.3.0 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/charithe/durationcheck v0.0.10 // indirect
	github.com/chavacava/garif v0.1.0 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/daixiang0/gci v0.12.3 // indirect
	github.com/denis-tingaikin/go-header v0.5.0 // indirect
	github.com/dgraph-io/badger/v2 v2.2007.4
	github.com/dgraph-io/ristretto v0.1.1 // indirect
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13 // indirect
	github.com/dnephin/pflag v1.0.7 // indirect
	github.com/ettle/strcase v0.2.0 // indirect
	github.com/fatih/structtag v1.2.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/fzipp/gocyclo v0.6.0 // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/go-critic/go-critic v0.11.2 // indirect
	github.com/go-kit/kit v0.12.0 // indirect
	github.com/go-playground/locales v0.14.0 // indirect
	github.com/go-playground/universal-translator v0.18.0 // indirect
	github.com/go-toolsmith/astcast v1.1.0 // indirect
	github.com/go-toolsmith/astcopy v1.1.0 // indirect
	github.com/go-toolsmith/astequal v1.2.0 // indirect
	github.com/go-toolsmith/astfmt v1.1.0 // indirect
	github.com/go-toolsmith/astp v1.1.0 // indirect
	github.com/go-toolsmith/strparse v1.1.0 // indirect
	github.com/go-toolsmith/typep v1.1.0 // indirect
	github.com/go-xmlfmt/xmlfmt v1.1.2 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/gofrs/flock v0.8.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v0.0.4
	github.com/golangci/dupl v0.0.0-20180902072040-3e9179ac440a // indirect
	github.com/golangci/gofmt v0.0.0-20231018234816-f50ced29576e // indirect
	github.com/golangci/misspell v0.4.1 // indirect
	github.com/golangci/revgrep v0.5.2 // indirect
	github.com/golangci/unconvert v0.0.0-20240309020433-c5143eacb3ed // indirect
	github.com/google/btree v1.1.2 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/orderedcode v0.0.1 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/gordonklaus/ineffassign v0.1.0 // indirect
	github.com/gorilla/websocket v1.5.1
	github.com/gostaticanalysis/analysisutil v0.7.1 // indirect
	github.com/gostaticanalysis/comment v1.4.2 // indirect
	github.com/gostaticanalysis/forcetypeassert v0.1.0 // indirect
	github.com/gostaticanalysis/nilerr v0.1.1 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-version v1.6.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hexops/gotextdiff v1.0.3 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jgautheron/goconst v1.7.0 // indirect
	github.com/jingyugao/rowserrcheck v1.1.1 // indirect
	github.com/jirfag/go-printf-func-name v0.0.0-20200119135958-7558a9eaa5af // indirect
	github.com/jmhodges/levigo v1.0.0 // indirect
	github.com/joho/godotenv v1.5.1
	github.com/jonboulle/clockwork v0.3.0 // indirect
	github.com/julz/importas v0.1.0 // indirect
	github.com/kisielk/errcheck v1.7.0 // indirect
	github.com/kulti/thelper v0.6.3 // indirect
	github.com/kunwardeep/paralleltest v1.0.10 // indirect
	github.com/kyoh86/exportloopref v0.1.11 // indirect
	github.com/ldez/gomoddirectives v0.2.3 // indirect
	github.com/ldez/tagliatelle v0.5.0 // indirect
	github.com/leodido/go-urn v1.2.1 // indirect
	github.com/leonklingele/grouper v1.1.1 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/maratori/testpackage v1.1.1 // indirect
	github.com/matoous/godox v0.0.0-20230222163458-006bad1f9d26 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/mgechev/revive v1.3.7 // indirect
	github.com/minio/highwayhash v1.0.2 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0
	github.com/moricho/tparallel v0.3.1 // indirect
	github.com/nakabonne/nestif v0.3.1 // indirect
	github.com/nishanths/exhaustive v0.12.0 // indirect
	github.com/nishanths/predeclared v0.2.2 // indirect
	github.com/olekukonko/tablewriter v0.0.5
	github.com/petermattis/goid v0.0.0-20180202154549-b0b1615b78e5 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/polyfloyd/go-errorlint v1.4.8 // indirect
	github.com/prometheus/client_model v0.6.1
	github.com/prometheus/procfs v0.15.0 // indirect
	github.com/quasilyte/go-ruleguard v0.4.2 // indirect
	github.com/quasilyte/gogrep v0.5.0 // indirect
	github.com/quasilyte/regex/syntax v0.0.0-20210819130434-b3f0c404a727 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475 // indirect
	github.com/rs/cors v1.9.0
	github.com/russross/blackfriday/v2 v2.1.0
	github.com/ryancurrah/gomodguard v1.3.0 // indirect
	github.com/ryanrolds/sqlclosecheck v0.5.1 // indirect
	github.com/sanposhiho/wastedassign/v2 v2.0.7 // indirect
	github.com/sasha-s/go-deadlock v0.3.1 // indirect
	github.com/securego/gosec/v2 v2.19.0 // indirect
	github.com/shazow/go-diff v0.0.0-20160112020656-b6b7b6733b8c // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/sivchari/containedctx v1.0.3 // indirect
	github.com/sivchari/tenv v1.7.1 // indirect
	github.com/sonatard/noctx v0.0.2 // indirect
	github.com/sourcegraph/go-diff v0.7.0 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/ssgreg/nlreturn/v2 v2.2.1 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/subosito/gotenv v1.4.2 // indirect
	github.com/syndtr/goleveldb v1.0.1-0.20210819022825-2ae1ddf74ef7
	github.com/tdakkota/asciicheck v0.2.0 // indirect
	github.com/tecbot/gorocksdb v0.0.0-20191217155057-f0fad39f321c // indirect
	github.com/tetafro/godot v1.4.16 // indirect
	github.com/timakin/bodyclose v0.0.0-20230421092635-574207250966 // indirect
	github.com/tomarrell/wrapcheck/v2 v2.8.3 // indirect
	github.com/tommy-muehle/go-mnd/v2 v2.5.1 // indirect
	github.com/ultraware/funlen v0.1.0 // indirect
	github.com/ultraware/whitespace v0.1.0 // indirect
	github.com/uudashr/gocognit v1.1.2 // indirect
	github.com/yagipy/maintidx v1.0.0 // indirect
	github.com/yeya24/promlinter v0.2.0 // indirect
	gitlab.com/accumulatenetwork/utils/jsonrpc v0.1.0
	gitlab.com/bosi/decorder v0.4.1 // indirect
	go.etcd.io/bbolt v1.3.6
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/crypto v0.23.0
	golang.org/x/mod v0.17.0 // indirect
	golang.org/x/net v0.25.0 // indirect
	google.golang.org/grpc v1.64.0 // indirect
	google.golang.org/protobuf v1.34.1
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/src-d/go-git.v4 v4.13.1
	gopkg.in/yaml.v2 v2.4.0 // indirect
	honnef.co/go/tools v0.4.7 // indirect
	mvdan.cc/gofumpt v0.6.0 // indirect
	mvdan.cc/unparam v0.0.0-20240104100049-c549a3470d14 // indirect
)
