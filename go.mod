module github.com/Financial-Times/annotations-rw-neo4j/v4

go 1.17

require (
	github.com/Financial-Times/cm-neo4j-driver v1.1.0
	github.com/Financial-Times/go-fthealth v0.0.0-20180807113633-3d8eb430d5b5
	github.com/Financial-Times/go-logger/v2 v2.0.1
	github.com/Financial-Times/http-handlers-go/v2 v2.1.0
	github.com/Financial-Times/kafka-client-go/v3 v3.0.4
	github.com/Financial-Times/neo-model-utils-go v0.0.0-20180712095719-aea1e95c8305
	github.com/Financial-Times/service-status-go v0.0.0-20160323111542-3f5199736a3d
	github.com/Financial-Times/transactionid-utils-go v0.2.0
	github.com/google/uuid v1.3.0
	github.com/gorilla/mux v1.8.0
	github.com/jawher/mow.cli v1.0.4
	github.com/pkg/errors v0.8.0
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475
	github.com/stretchr/testify v1.7.0
)

require (
	github.com/Shopify/sarama v1.33.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/eapache/go-resiliency v1.2.0 // indirect
	github.com/eapache/go-xerial-snappy v0.0.0-20180814174437-776d5712da21 // indirect
	github.com/eapache/queue v1.1.0 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-uuid v1.0.2 // indirect
	github.com/hashicorp/go-version v1.0.0 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.0.0 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.2 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/klauspost/compress v1.15.0 // indirect
	github.com/neo4j/neo4j-go-driver/v4 v4.3.3 // indirect
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/stretchr/objx v0.1.1 // indirect
	golang.org/x/crypto v0.0.0-20220214200702-86341886e292 // indirect
	golang.org/x/net v0.0.0-20220225172249-27dd8689420f // indirect
	golang.org/x/sys v0.0.0-20211216021012-1d35b9e2eb4e // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)

replace gopkg.in/stretchr/testify.v1 => github.com/stretchr/testify v1.4.0
