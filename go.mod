module github.com/vmware/antrea-operator-for-kubernetes

go 1.16

require (
	github.com/containerd/go-cni v1.0.1
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/go-logr/logr v0.4.0
	github.com/onsi/gomega v1.13.0
	github.com/openshift/api v0.0.0-20211203184245-fb6f933bb8d5
	github.com/openshift/cluster-network-operator v0.0.0-20220125073742-83efd7f8d510
	github.com/openshift/library-go v0.0.0-20210708173104-7e7d216ed91c
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	k8s.io/component-base v0.22.1
	k8s.io/utils v0.0.0-20210820185131-d34e5cb4466e
	sigs.k8s.io/controller-runtime v0.9.0-beta.0
)

replace (
	github.com/dgrijalva/jwt-go => github.com/golang-jwt/jwt v3.2.1+incompatible
	sigs.k8s.io/cluster-api-provider-aws => github.com/openshift/cluster-api-provider-aws v0.2.1-0.20201125052318-b85a18cbf338
	sigs.k8s.io/cluster-api-provider-azure => github.com/openshift/cluster-api-provider-azure v0.1.0-alpha.3.0.20201130182513-88b90230f2a4
	sigs.k8s.io/cluster-api-provider-openstack => github.com/openshift/cluster-api-provider-openstack v0.0.0-20210107201226-5f60693f7a71
)
