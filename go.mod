module github.com/vmware/antrea-operator-for-kubernetes

go 1.13

require (
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/go-logr/logr v0.1.0
	github.com/onsi/gomega v1.10.1
	github.com/openshift/api v0.0.0-20200701144905-de5b010b2b38
	github.com/openshift/cluster-network-operator v0.0.0-20200820075439-92e466db53cc
	gopkg.in/yaml.v2 v2.3.0
	k8s.io/api v0.18.6
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v12.0.0+incompatible
	sigs.k8s.io/controller-runtime v0.6.3
)

replace k8s.io/client-go => k8s.io/client-go v0.18.6
