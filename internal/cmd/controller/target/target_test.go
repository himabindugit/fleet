package target

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/rancher/wrangler/v2/pkg/generic/fake"
	"github.com/rancher/wrangler/v2/pkg/yaml"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
)

const bundleYaml = `namespace: default
helm:
  releaseName: labels
  values:
    clusterName: global.fleet.clusterLabels.name
    customStruct:
      - name: global.fleet.clusterLabels.name
        key1: value1
        key2: value2
      - element1: global.fleet.clusterLabels.envType
      - element2: global.fleet.clusterLabels.name
diff:
  comparePatches:
  - apiVersion: networking.k8s.io/v1
    kind: Ingress
    name: labels-fleetlabelsdemo
    namespace: default
    operations:
    - op: remove
      path: /spec/rules/0/host
`

func TestProcessLabelValues(t *testing.T) {

	bundle := &v1alpha1.BundleSpec{}

	clusterLabels := make(map[string]string)
	clusterLabels["name"] = "local"
	clusterLabels["envType"] = "dev"

	err := yaml.Unmarshal([]byte(bundleYaml), bundle)
	if err != nil {
		t.Fatalf("error during yaml parsing %v", err)
	}

	err = processLabelValues(bundle.Helm.Values.Data, clusterLabels, 0)
	if err != nil {
		t.Fatalf("error during label processing %v", err)
	}

	clusterName, ok := bundle.Helm.Values.Data["clusterName"]
	if !ok {
		t.Fatal("key clusterName not found")
	}

	if clusterName != "local" {
		t.Fatal("unable to assert correct clusterName")
	}

	customStruct, ok := bundle.Helm.Values.Data["customStruct"].([]interface{})
	if !ok {
		t.Fatal("key customStruct not found")
	}

	firstMap, ok := customStruct[0].(map[string]interface{})
	if !ok {
		t.Fatal("unable to assert first element to map[string]interface{}")
	}

	firstElemVal, ok := firstMap["name"]
	if !ok {
		t.Fatal("unable to find key name in the first element of customStruct")
	}

	if firstElemVal.(string) != "local" {
		t.Fatal("label replacement not performed in first element")
	}

	secondElement, ok := customStruct[1].(map[string]interface{})
	if !ok {
		t.Fatal("unable to assert second element of customStruct to map[string]interface{}")
	}

	secondElemVal, ok := secondElement["element1"]
	if !ok {
		t.Fatal("unable to find key element1")
	}

	if secondElemVal.(string) != "dev" {
		t.Fatal("label replacement not performed in second element")
	}

	thirdElement, ok := customStruct[2].(map[string]interface{})
	if !ok {
		t.Fatal("unable to assert third element of customStruct to map[string]interface{}")
	}

	thirdElemVal, ok := thirdElement["element2"]
	if !ok {
		t.Fatal("unable to find key element2")
	}

	if thirdElemVal.(string) != "local" {
		t.Fatal("label replacement not performed in third element")
	}
}

const bundleYamlWithTemplate = `namespace: default
helm:
  releaseName: labels
  values:
    clusterName: "${ .ClusterLabels.name }"
    fromAnnotation: "${ .ClusterAnnotations.testAnnotation }"
    clusterNamespace: "${ .ClusterNamespace }"
    fleetClusterName: "${ .ClusterName }"
    reallyLongClusterName: kubernets.io/cluster/${ index .ClusterLabels "really-long-label-name-with-many-many-characters-in-it" }
    missingLabel: |-
      ${ if hasKey .ClusterLabels "missing" }${ .ClusterLabels.missing }${ else }missing${ end}
    list: ${ list 1 2 3 | toJson }
    listb: |-
      ${- range $key, $val := .ClusterLabels }
      - name: ${ $key }
        value: ${ $val | quote }
      ${- end}
    customStruct:
      - name: "${ .ClusterValues.topLevel }"
        key1: value1
        key2: value2
      - element2: "${ .ClusterValues.nested.secondTier.thirdTier }"
      - "element3_${ .ClusterLabels.envType }": "${ .ClusterLabels.name }"
    funcs:
      upper: "${ .ClusterValues.topLevel | upper }_test"
      join: '${ .ClusterValues.list | join "," }'
diff:
  comparePatches:
  - apiVersion: networking.k8s.io/v1
    kind: Ingress
    name: labels-fleetlabelsdemo
    namespace: default
    operations:
    - op: remove
      path: /spec/rules/0/host
`

func TestProcessTemplateValues(t *testing.T) {
	templateValues := map[string]interface{}{
		"topLevel": "foo",
		"nested": map[string]interface{}{
			"secondTier": map[string]interface{}{
				"thirdTier": "bar",
			},
		},
		"list": []string{
			"alpha",
			"beta",
			"omega",
		},
	}

	clusterLabels := map[string]interface{}{
		"name":    "local",
		"envType": "dev",
		"really-long-label-name-with-many-many-characters-in-it": "foobar",
	}

	clusterAnnotations := map[string]interface{}{
		"testAnnotation": "test",
	}

	values := map[string]interface{}{
		"ClusterNamespace":   "dev-clusters",
		"ClusterName":        "my-cluster",
		"ClusterLabels":      clusterLabels,
		"ClusterAnnotations": clusterAnnotations,
		"ClusterValues":      templateValues,
	}

	bundle := &v1alpha1.BundleSpec{}
	err := yaml.Unmarshal([]byte(bundleYamlWithTemplate), bundle)
	if err != nil {
		t.Fatalf("error during yaml parsing %v", err)
	}

	templatedValues, err := processTemplateValues(bundle.Helm.Values.Data, values)
	if err != nil {
		t.Fatalf("error during label processing %v", err)
	}

	clusterName, ok := templatedValues["clusterName"]
	if !ok {
		t.Fatal("key clusterName not found")
	}

	if clusterName != "local" {
		t.Fatal("unable to assert correct clusterName")
	}

	fromAnnotation, ok := templatedValues["fromAnnotation"]
	if !ok {
		t.Fatal("key fromAnnotation not found")
	}

	if fromAnnotation != "test" {
		t.Fatal("unable to assert correct value for fromAnnotation")
	}

	clusterNamespace, ok := templatedValues["clusterNamespace"]
	if !ok {
		t.Fatal("key clusterNamespace not found")
	}

	if clusterNamespace != "dev-clusters" {
		t.Fatal("unable to assert correct value for clusterNamespace")
	}

	fleetClusterName, ok := templatedValues["fleetClusterName"]
	if !ok {
		t.Fatal("key clusterName not found")
	}

	if fleetClusterName != "my-cluster" {
		t.Fatal("unable to assert correct value fleetClusterName")
	}

	reallyLongClusterName, ok := templatedValues["reallyLongClusterName"]
	if !ok {
		t.Fatal("key reallyLongClusterName not found")
	}

	if reallyLongClusterName != "kubernets.io/cluster/foobar" {
		t.Fatal("unable to assert correct value reallyLongClusterName")
	}

	missingLabel, ok := templatedValues["missingLabel"]
	if !ok {
		t.Fatal("key missingLabel not found")
	}

	if missingLabel != "missing" {
		t.Fatal("unable to assert correct value missingLabel: ", missingLabel)
	}

	customStruct, ok := templatedValues["customStruct"].([]interface{})
	if !ok {
		t.Fatal("key customStruct not found")
	}

	firstMap, ok := customStruct[0].(map[string]interface{})
	if !ok {
		t.Fatal("unable to assert first element to map[string]interface{}")
	}

	firstElemVal, ok := firstMap["name"]
	if !ok {
		t.Fatal("unable to find key name in the first element of customStruct")
	}

	if firstElemVal.(string) != "foo" {
		t.Fatal("label replacement not performed in first element")
	}

	secondElement, ok := customStruct[1].(map[string]interface{})
	if !ok {
		t.Fatal("unable to assert second element of customStruct to map[string]interface{}")
	}

	secondElemVal, ok := secondElement["element2"]
	if !ok {
		t.Fatal("unable to find key element2")
	}

	if secondElemVal.(string) != "bar" {
		t.Fatal("template replacement not performed in second element")
	}

	thirdElement, ok := customStruct[2].(map[string]interface{})
	if !ok {
		t.Fatal("unable to assert second element of customStruct to map[string]interface{}")
	}

	thirdElemVal, ok := thirdElement["element3_dev"]
	if !ok {
		t.Fatal("unable to find key element3_dev")
	}

	if thirdElemVal.(string) != "local" {
		t.Fatal("template replacement not performed in third element")
	}

	funcs, ok := templatedValues["funcs"].(map[string]interface{})
	if !ok {
		t.Fatal("key funcs not found")
	}

	upper, ok := funcs["upper"]
	if !ok {
		t.Fatal("key upper not found")
	}

	if upper.(string) != "FOO_test" {
		t.Fatal("upper func was not right")
	}

	join, ok := funcs["join"]
	if !ok {
		t.Fatal("key join not found")
	}

	if join.(string) != "alpha,beta,omega" {
		t.Fatal("join func was not right")
	}

}

const clusterYamlWithTemplateValues = `apiVersion: fleet.cattle.io/v1alpha1
kind: Cluster
metadata:
  name: test-cluster
  namespace: test-namespace
  labels:
    testLabel: test-label-value
spec:
  templateValues:
    someKey: someValue
`

func getClusterAndBundle(bundleYaml string) (*v1alpha1.Cluster, *v1alpha1.BundleDeploymentOptions, error) {
	cluster := &v1alpha1.Cluster{}
	err := yaml.Unmarshal([]byte(clusterYamlWithTemplateValues), cluster)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error during cluster yaml parsing")
	}

	bundle := &v1alpha1.BundleDeploymentOptions{}
	err = yaml.Unmarshal([]byte(bundleYaml), bundle)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error during bundle yaml parsing")
	}

	return cluster, bundle, nil
}

const bundleYamlWithDisablePreProcessEnabled = `namespace: default
helm:
  disablePreprocess: true
  releaseName: labels
  values:
    clusterName: "${ .ClusterName }"
    clusterContext: "${ .Values.someKey }"
    templateFn: '${ index .ClusterLabels "testLabel" }'
    syntaxError: "${ non_existent_function }"
`

func TestDisablePreProcessFlagEnabled(t *testing.T) {
	cluster, bundle, err := getClusterAndBundle(bundleYamlWithDisablePreProcessEnabled)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = preprocessHelmValues(bundle, cluster)
	if err != nil {
		t.Fatalf("error during cluster processing %v", err)
	}

	valuesObj := bundle.Helm.Values.Data

	for _, testCase := range []struct {
		Key           string
		ExpectedValue string
	}{
		{
			Key:           "clusterName",
			ExpectedValue: "${ .ClusterName }",
		},
		{
			Key:           "clusterContext",
			ExpectedValue: "${ .Values.someKey }",
		},
		{
			Key:           "templateFn",
			ExpectedValue: "${ index .ClusterLabels \"testLabel\" }",
		},
		{
			Key:           "syntaxError",
			ExpectedValue: "${ non_existent_function }",
		},
	} {
		if field, ok := valuesObj[testCase.Key]; !ok {
			t.Fatalf("key %s not found", testCase.Key)
		} else {
			if field != testCase.ExpectedValue {
				t.Fatalf("key %s was not the expected value. Expected: '%s' Actual: '%s'", testCase.Key, field, testCase.ExpectedValue)
			}
		}

	}

}

const bundleYamlWithDisablePreProcessDisabled = `namespace: default
helm:
  disablePreprocess: false
  releaseName: labels
  values:
    clusterName: "${ .ClusterName }"
`

func TestDisablePreProcessFlagDisabled(t *testing.T) {
	cluster, bundle, err := getClusterAndBundle(bundleYamlWithDisablePreProcessDisabled)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = preprocessHelmValues(bundle, cluster)
	if err != nil {
		t.Fatalf("error during cluster processing %v", err)
	}

	valuesObj := bundle.Helm.Values.Data

	key := "clusterName"
	expectedValue := "test-cluster"

	if field, ok := valuesObj[key]; !ok {
		t.Fatalf("key %s not found", key)
	} else {
		if field != expectedValue {
			t.Fatalf("key %s was not the expected value. Expected: '%s' Actual: '%s'", key, field, expectedValue)
		}
	}

}

const bundleYamlWithDisablePreProcessMissing = `namespace: default
helm:
  releaseName: labels
  values:
    clusterName: "${ .ClusterName }"
`

func TestDisablePreProcessFlagMissing(t *testing.T) {
	cluster, bundle, err := getClusterAndBundle(bundleYamlWithDisablePreProcessMissing)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = preprocessHelmValues(bundle, cluster)
	if err != nil {
		t.Fatalf("error during cluster processing %v", err)
	}

	valuesObj := bundle.Helm.Values.Data

	key := "clusterName"
	expectedValue := "test-cluster"

	if field, ok := valuesObj[key]; !ok {
		t.Fatalf("key %s not found", key)
	} else {
		if field != expectedValue {
			t.Fatalf("key %s was not the expected value. Expected: '%s' Actual: '%s'", key, field, expectedValue)
		}
	}

}

func TestGetBundleDeploymentForBundleInCluster(t *testing.T) {
	bundleDeployment := func(namespace, clusterName string) *v1alpha1.BundleDeployment {
		return &v1alpha1.BundleDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: fmt.Sprintf("cluster-%s-%s-1df72965a9b5", namespace, clusterName),
				Labels: map[string]string{
					"fleet.cattle.io/cluster": clusterName,
				},
			},
		}
	}
	bundle := func(name, namespace string) *v1alpha1.Bundle {
		return &v1alpha1.Bundle{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}
	}
	cluster := func(name string) *v1alpha1.Cluster {
		return &v1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		}
	}
	bundleName := "my-bundle"
	bundleNamespace := "fleet-default"
	clusterName := "my-cluster"
	bundleDeployments := []*v1alpha1.BundleDeployment{
		bundleDeployment(bundleNamespace, clusterName),
		bundleDeployment(bundleNamespace, "another-cluster"),
	}

	testCases := []struct {
		name                       string
		bundleName                 string
		bundleNamespace            string
		clusterName                string
		listBundleDeploymentsError error
		expectedBundleDeployments  []*v1alpha1.BundleDeployment
		wantError                  bool
	}{
		{
			name:            "returns listed bundle deployments",
			bundleName:      bundleName,
			bundleNamespace: bundleNamespace,
			clusterName:     clusterName,
			expectedBundleDeployments: []*v1alpha1.BundleDeployment{
				bundleDeployment(bundleNamespace, clusterName),
			},
		},
		{
			name:                       "returns error from bundle deployment cache listing",
			bundleName:                 bundleName,
			bundleNamespace:            bundleNamespace,
			listBundleDeploymentsError: errors.New("something happened"),
			expectedBundleDeployments:  nil,
			wantError:                  true,
		},
		{
			name:                      "returns no bundle deployments when none are listed",
			bundleName:                bundleName,
			bundleNamespace:           bundleNamespace,
			clusterName:               "yet-another-cluster",
			expectedBundleDeployments: []*v1alpha1.BundleDeployment{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockBundleDeploymentCache := fake.NewMockCacheInterface[*v1alpha1.BundleDeployment](ctrl)

			mockBundleDeploymentCache.EXPECT().AddIndexer(byBundleIndexerName, gomock.Any())
			mockBundleDeploymentCache.EXPECT().GetByIndex(byBundleIndexerName, fmt.Sprintf("%s/%s", tc.bundleNamespace, tc.bundleName)).
				Return(bundleDeployments, tc.listBundleDeploymentsError)

			manager := New(nil, nil, nil, nil, nil, nil, mockBundleDeploymentCache)
			result, err := manager.GetBundleDeploymentsForBundleInCluster(bundle(tc.bundleName, tc.bundleNamespace), cluster(tc.clusterName))

			if tc.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.expectedBundleDeployments, result)
		})
	}
}

func newManagerWithClusterGroupCache(t *testing.T, ctrl *gomock.Controller, cgCache *fake.MockCacheInterface[*v1alpha1.ClusterGroup]) *Manager {
	t.Helper()
	mockBDCache := fake.NewMockCacheInterface[*v1alpha1.BundleDeployment](ctrl)
	mockBDCache.EXPECT().AddIndexer(byBundleIndexerName, gomock.Any())
	return New(nil, cgCache, nil, nil, nil, nil, mockBDCache)
}

func TestClusterGroupsForCluster_Matching(t *testing.T) {
	makeCG := func(name, rv string, selector *metav1.LabelSelector) *v1alpha1.ClusterGroup {
		return &v1alpha1.ClusterGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Namespace:       "fleet-default",
				ResourceVersion: rv,
			},
			Spec: v1alpha1.ClusterGroupSpec{
				Selector: selector,
			},
		}
	}

	testCases := []struct {
		name          string
		cgs           []*v1alpha1.ClusterGroup
		clusterLabels map[string]string
		listError     error
		expectedNames []string
		wantError     bool
	}{
		{
			name: "matching cluster group returned",
			cgs: []*v1alpha1.ClusterGroup{
				makeCG("prod-cg", "1", &metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "prod"},
				}),
			},
			clusterLabels: map[string]string{"env": "prod"},
			expectedNames: []string{"prod-cg"},
		},
		{
			name: "non-matching cluster group excluded",
			cgs: []*v1alpha1.ClusterGroup{
				makeCG("prod-cg", "1", &metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "prod"},
				}),
			},
			clusterLabels: map[string]string{"env": "staging"},
			expectedNames: []string{},
		},
		{
			name: "nil selector cluster group excluded",
			cgs: []*v1alpha1.ClusterGroup{
				makeCG("no-selector-cg", "1", nil),
			},
			clusterLabels: map[string]string{"env": "prod"},
			expectedNames: []string{},
		},
		{
			name: "invalid selector skipped",
			cgs: []*v1alpha1.ClusterGroup{
				makeCG("bad-cg", "1", &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{Key: "env", Operator: "InvalidOp", Values: []string{"prod"}},
					},
				}),
			},
			clusterLabels: map[string]string{"env": "prod"},
			expectedNames: []string{},
		},
		{
			name:          "list error propagated",
			listError:     errors.New("list failed"),
			clusterLabels: nil,
			wantError:     true,
		},
		{
			name: "multiple cgs, only matching returned",
			cgs: []*v1alpha1.ClusterGroup{
				makeCG("prod-cg", "1", &metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "prod"},
				}),
				makeCG("staging-cg", "1", &metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "staging"},
				}),
			},
			clusterLabels: map[string]string{"env": "prod"},
			expectedNames: []string{"prod-cg"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockCGCache := fake.NewMockCacheInterface[*v1alpha1.ClusterGroup](ctrl)
			mockCGCache.EXPECT().List("fleet-default", gomock.Any()).Return(tc.cgs, tc.listError)

			cluster := &v1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "fleet-default",
					Labels:    tc.clusterLabels,
				},
			}

			manager := newManagerWithClusterGroupCache(t, ctrl, mockCGCache)
			result, err := manager.clusterGroupsForCluster(cluster)

			if tc.wantError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			names := make([]string, len(result))
			for i, cg := range result {
				names[i] = cg.Name
			}
			if len(tc.expectedNames) == 0 {
				assert.Empty(t, names)
			} else {
				assert.Equal(t, tc.expectedNames, names)
			}
		})
	}
}

func TestClusterGroupsForCluster_SelectorCachedAfterFirstCall(t *testing.T) {
	// After the first call the selector is stored in selectorCache keyed by
	// namespace/name@resourceVersion; a second call must find it.
	ctrl := gomock.NewController(t)

	cg := &v1alpha1.ClusterGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "prod-cg",
			Namespace:       "fleet-default",
			ResourceVersion: "42",
		},
		Spec: v1alpha1.ClusterGroupSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"env": "prod"},
			},
		},
	}

	cluster := &v1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cluster",
			Namespace: "fleet-default",
			Labels:    map[string]string{"env": "prod"},
		},
	}

	mockCGCache := fake.NewMockCacheInterface[*v1alpha1.ClusterGroup](ctrl)
	// List is called on each clusterGroupsForCluster invocation; selector compilation is cached.
	mockCGCache.EXPECT().List("fleet-default", gomock.Any()).Return([]*v1alpha1.ClusterGroup{cg}, nil).Times(2)

	manager := newManagerWithClusterGroupCache(t, ctrl, mockCGCache)

	result1, err := manager.clusterGroupsForCluster(cluster)
	assert.NoError(t, err)
	assert.Len(t, result1, 1)

	cacheKey := "fleet-default/prod-cg@42"
	cached, ok := manager.selectorCache.Load(cacheKey)
	assert.True(t, ok, "selector should be in cache after first call")
	assert.Implements(t, (*labels.Selector)(nil), cached)

	result2, err := manager.clusterGroupsForCluster(cluster)
	assert.NoError(t, err)
	assert.Len(t, result2, 1)
}

func TestClusterGroupsForCluster_NewResourceVersionGetsNewCacheEntry(t *testing.T) {
	// When a ClusterGroup is updated (ResourceVersion bumped), a new cache entry is
	// created so the updated selector is compiled rather than served stale.
	ctrl := gomock.NewController(t)

	makeCG := func(rv string) *v1alpha1.ClusterGroup {
		return &v1alpha1.ClusterGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "prod-cg",
				Namespace:       "fleet-default",
				ResourceVersion: rv,
			},
			Spec: v1alpha1.ClusterGroupSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "prod"},
				},
			},
		}
	}

	cluster := &v1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cluster",
			Namespace: "fleet-default",
			Labels:    map[string]string{"env": "prod"},
		},
	}

	mockCGCache := fake.NewMockCacheInterface[*v1alpha1.ClusterGroup](ctrl)
	mockCGCache.EXPECT().List("fleet-default", gomock.Any()).Return([]*v1alpha1.ClusterGroup{makeCG("1")}, nil)
	mockCGCache.EXPECT().List("fleet-default", gomock.Any()).Return([]*v1alpha1.ClusterGroup{makeCG("2")}, nil)

	manager := newManagerWithClusterGroupCache(t, ctrl, mockCGCache)

	_, err := manager.clusterGroupsForCluster(cluster)
	assert.NoError(t, err)

	_, err = manager.clusterGroupsForCluster(cluster)
	assert.NoError(t, err)

	_, v1Cached := manager.selectorCache.Load("fleet-default/prod-cg@1")
	_, v2Cached := manager.selectorCache.Load("fleet-default/prod-cg@2")
	assert.True(t, v1Cached, "entry for rv=1 should be cached")
	assert.True(t, v2Cached, "entry for rv=2 should be cached as a new entry")
}

func TestClusterGroupsForCluster_InvalidSelectorNotCached(t *testing.T) {
	// A ClusterGroup with an invalid selector must not be stored in the cache.
	ctrl := gomock.NewController(t)

	cg := &v1alpha1.ClusterGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "bad-cg",
			Namespace:       "fleet-default",
			ResourceVersion: "1",
		},
		Spec: v1alpha1.ClusterGroupSpec{
			Selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{Key: "env", Operator: "InvalidOp", Values: []string{"prod"}},
				},
			},
		},
	}

	cluster := &v1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cluster",
			Namespace: "fleet-default",
			Labels:    map[string]string{"env": "prod"},
		},
	}

	mockCGCache := fake.NewMockCacheInterface[*v1alpha1.ClusterGroup](ctrl)
	mockCGCache.EXPECT().List("fleet-default", gomock.Any()).Return([]*v1alpha1.ClusterGroup{cg}, nil)

	manager := newManagerWithClusterGroupCache(t, ctrl, mockCGCache)

	result, err := manager.clusterGroupsForCluster(cluster)
	assert.NoError(t, err)
	assert.Empty(t, result)

	_, cached := manager.selectorCache.Load("fleet-default/bad-cg@1")
	assert.False(t, cached, "invalid selector must not be stored in cache")
}
