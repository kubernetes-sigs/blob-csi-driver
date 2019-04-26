module github.com/csi-driver/blobfuse-csi-driver

go 1.12

require (
	contrib.go.opencensus.io/exporter/ocagent v0.2.0 // indirect
	github.com/Azure/azure-sdk-for-go v21.3.0+incompatible
	github.com/Azure/go-autorest v11.5.1+incompatible // indirect
	github.com/container-storage-interface/spec v1.0.0
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgrijalva/jwt-go v3.2.0+incompatible // indirect
	github.com/dnaeon/go-vcr v1.0.1 // indirect
	github.com/docker/distribution v2.6.0-rc.1.0.20170905204447-5db89f0ca686+incompatible // indirect
	github.com/gogo/protobuf v1.2.1 // indirect
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/golang/protobuf v1.2.0
	github.com/google/gofuzz v0.0.0-20170612174753-24818f796faf // indirect
	github.com/googleapis/gnostic v0.2.0 // indirect
	github.com/hashicorp/golang-lru v0.5.1 // indirect
	github.com/json-iterator/go v1.1.6 // indirect
	github.com/kubernetes-csi/csi-test v1.1.0
	github.com/marstr/guid v1.1.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/onsi/ginkgo v1.8.0 // indirect
	github.com/onsi/gomega v1.5.0 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/pborman/uuid v1.2.0
	github.com/rubiojr/go-vhd v0.0.0-20160810183302-0bfd3b39853c // indirect
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/spf13/afero v1.2.1 // indirect
	github.com/spf13/pflag v1.0.3 // indirect
	github.com/stretchr/testify v1.3.0
	go.opencensus.io v0.18.0 // indirect
	golang.org/x/crypto v0.0.0-20190313024323-a1f597ede03a // indirect
	golang.org/x/net v0.0.0-20180906233101-161cd47e91fd
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4 // indirect
	google.golang.org/grpc v1.15.0
	gopkg.in/inf.v0 v0.9.1 // indirect
	k8s.io/api v0.0.0-20190313235455-40a48860b5ab // indirect
	k8s.io/apiextensions-apiserver v0.0.0-20190315093550-53c4693659ed // indirect
	k8s.io/apimachinery v0.0.0-20190313205120-d7deff9243b1 // indirect
	k8s.io/apiserver v0.0.0-20190313205120-8b27c41bdbb1 // indirect
	k8s.io/client-go v11.0.0+incompatible // indirect
	k8s.io/cloud-provider v0.0.0-20190313124351-c76aa0a348b5 // indirect
	k8s.io/klog v0.2.0
	k8s.io/kube-openapi v0.0.0-20190320154901-5e45bb682580 // indirect
	k8s.io/kubernetes v1.14.0
	k8s.io/utils v0.0.0-20190308190857-21c4ce38f2a7 // indirect
	sigs.k8s.io/yaml v1.1.0
)

replace (
	cloud.google.com/go => cloud.google.com/go v0.26.0
	contrib.go.opencensus.io/exporter/ocagent => contrib.go.opencensus.io/exporter/ocagent v0.2.0
	git.apache.org/thrift.git => git.apache.org/thrift.git v0.0.0-20180902110319-2566ecd5d999
	github.com/Azure/azure-sdk-for-go => github.com/Azure/azure-sdk-for-go v21.3.0+incompatible
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v11.5.1+incompatible
	github.com/beorn7/perks => github.com/beorn7/perks v0.0.0-20180321164747-3a771d992973
	github.com/census-instrumentation/opencensus-proto => github.com/census-instrumentation/opencensus-proto v0.0.2-0.20180913191712-f303ae3f8d6a
	github.com/client9/misspell => github.com/client9/misspell v0.3.4
	github.com/container-storage-interface/spec => github.com/container-storage-interface/spec v1.0.0
	github.com/davecgh/go-spew => github.com/davecgh/go-spew v1.1.1
	github.com/dgrijalva/jwt-go => github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/dnaeon/go-vcr => github.com/dnaeon/go-vcr v1.0.1
	github.com/docker/distribution => github.com/docker/distribution v2.6.0-rc.1.0.20170905204447-5db89f0ca686+incompatible
	github.com/fsnotify/fsnotify => github.com/fsnotify/fsnotify v1.4.7
	github.com/ghodss/yaml => github.com/ghodss/yaml v1.0.0
	github.com/gogo/protobuf => github.com/gogo/protobuf v1.2.1
	github.com/golang/glog => github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/groupcache => github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef
	github.com/golang/lint => github.com/golang/lint v0.0.0-20180702182130-06c8688daad7
	github.com/golang/mock => github.com/golang/mock v1.1.1
	github.com/golang/protobuf => github.com/golang/protobuf v1.2.0
	github.com/google/go-cmp => github.com/google/go-cmp v0.2.0
	github.com/google/gofuzz => github.com/google/gofuzz v0.0.0-20170612174753-24818f796faf
	github.com/google/uuid => github.com/google/uuid v1.0.0
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.2.0
	github.com/grpc-ecosystem/grpc-gateway => github.com/grpc-ecosystem/grpc-gateway v1.5.0
	github.com/hashicorp/golang-lru => github.com/hashicorp/golang-lru v0.5.1
	github.com/hpcloud/tail => github.com/hpcloud/tail v1.0.0
	github.com/json-iterator/go => github.com/json-iterator/go v1.1.6
	github.com/kisielk/errcheck => github.com/kisielk/errcheck v1.1.0
	github.com/kisielk/gotool => github.com/kisielk/gotool v1.0.0
	github.com/kubernetes-csi/csi-test => github.com/kubernetes-csi/csi-test v1.1.0
	github.com/marstr/guid => github.com/marstr/guid v1.1.0
	github.com/matttproud/golang_protobuf_extensions => github.com/matttproud/golang_protobuf_extensions v1.0.1
	github.com/modern-go/concurrent => github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd
	github.com/modern-go/reflect2 => github.com/modern-go/reflect2 v1.0.1
	github.com/onsi/ginkgo => github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega => github.com/onsi/gomega v1.5.0
	github.com/opencontainers/go-digest => github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/openzipkin/zipkin-go => github.com/openzipkin/zipkin-go v0.1.1
	github.com/pborman/uuid => github.com/pborman/uuid v1.2.0
	github.com/pmezard/go-difflib => github.com/pmezard/go-difflib v1.0.0
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.8.0
	github.com/prometheus/client_model => github.com/prometheus/client_model v0.0.0-20180712105110-5c3871d89910
	github.com/prometheus/common => github.com/prometheus/common v0.0.0-20180801064454-c7de2306084e
	github.com/prometheus/procfs => github.com/prometheus/procfs v0.0.0-20180725123919-05ee40e3a273
	github.com/rubiojr/go-vhd => github.com/rubiojr/go-vhd v0.0.0-20160810183302-0bfd3b39853c
	github.com/satori/go.uuid => github.com/satori/go.uuid v1.2.0
	github.com/spf13/afero => github.com/spf13/afero v1.2.1
	github.com/spf13/pflag => github.com/spf13/pflag v1.0.3
	github.com/stretchr/objx => github.com/stretchr/objx v0.1.0
	github.com/stretchr/testify => github.com/stretchr/testify v1.3.0
	go.opencensus.io => go.opencensus.io v0.18.0
	golang.org/x/crypto => golang.org/x/crypto v0.0.0-20190313024323-a1f597ede03a
	golang.org/x/lint => golang.org/x/lint v0.0.0-20180702182130-06c8688daad7
	golang.org/x/net => golang.org/x/net v0.0.0-20180906233101-161cd47e91fd
	golang.org/x/oauth2 => golang.org/x/oauth2 v0.0.0-20180821212333-d2e6202438be
	golang.org/x/sync => golang.org/x/sync v0.0.0-20180314180146-1d60e4601c6f
	golang.org/x/sys => golang.org/x/sys v0.0.0-20190215142949-d0b11bdaac8a
	golang.org/x/text => golang.org/x/text v0.3.0
	golang.org/x/time => golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
	golang.org/x/tools => golang.org/x/tools v0.0.0-20180828015842-6cd1fcedba52
	google.golang.org/api => google.golang.org/api v0.0.0-20180910000450-7ca32eb868bf
	google.golang.org/appengine => google.golang.org/appengine v1.1.0
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20180831171423-11092d34479b
	google.golang.org/grpc => google.golang.org/grpc v1.15.0
	gopkg.in/check.v1 => gopkg.in/check.v1 v0.0.0-20161208181325-20d25e280405
	gopkg.in/fsnotify.v1 => gopkg.in/fsnotify.v1 v1.4.7
	gopkg.in/inf.v0 => gopkg.in/inf.v0 v0.9.1
	gopkg.in/tomb.v1 => gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7
	gopkg.in/yaml.v2 => gopkg.in/yaml.v2 v2.2.1
	honnef.co/go/tools => honnef.co/go/tools v0.0.0-20180728063816-88497007e858
	k8s.io/api => k8s.io/api v0.0.0-20190313235455-40a48860b5ab
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190315093550-53c4693659ed
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190313205120-d7deff9243b1
	k8s.io/apiserver => k8s.io/apiserver v0.0.0-20190313205120-8b27c41bdbb1
	k8s.io/client-go => k8s.io/client-go v11.0.0+incompatible
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.0.0-20190313124351-c76aa0a348b5
	k8s.io/klog => k8s.io/klog v0.2.0
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20190320154901-5e45bb682580
	k8s.io/kubernetes => k8s.io/kubernetes v1.14.0
	k8s.io/utils => k8s.io/utils v0.0.0-20190308190857-21c4ce38f2a7
	sigs.k8s.io/yaml => sigs.k8s.io/yaml v1.1.0
)
