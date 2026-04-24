const test = require('node:test');
const assert = require('node:assert/strict');

const schemaSearch = require('./schema_search.js');

function table(name, cases, fn) {
  test(name, async (t) => {
    for (const tt of cases) {
      await t.test(tt.name, () => fn(tt));
    }
  });
}

table('matchesPathQuery', [
  {
    name: 'exact path query matches exact node',
    path: 'spec.acme',
    query: '.spec.acme',
    want: true,
  },
  {
    name: 'exact path query does not match descendants',
    path: 'spec.acme.settings',
    query: '.spec.acme',
    want: false,
  },
  {
    name: 'partial final segment matches same-depth path',
    path: 'spec.template',
    query: '.spec.tem',
    want: true,
  },
  {
    name: 'partial final segment does not match deeper descendants',
    path: 'spec.template.spec',
    query: '.spec.tem',
    want: false,
  },
  {
    name: 'trailing dot matches immediate children',
    path: 'spec.template',
    query: '.spec.',
    want: true,
  },
  {
    name: 'trailing dot does not match deeper descendants',
    path: 'spec.template.spec',
    query: '.spec.',
    want: false,
  },
  {
    name: 'matching is case insensitive',
    path: 'spec.volumeClaimTemplates',
    query: '.spec.volumeclaimt',
    want: true,
  },
  {
    name: 'array child path matches exact query',
    path: 'spec.ipam.pools.allocated[].cidrs',
    query: '.spec.ipam.pools.allocated[].cidrs',
    want: true,
  },
  {
    name: 'virtual array item query matches immediate children',
    path: 'spec.ipam.pools.allocated[].cidrs',
    query: '.spec.ipam.pools.allocated[]',
    want: true,
  },
], ({ path, query, want }) => {
  assert.equal(schemaSearch.matchesPathQuery(path, query), want);
});

table('completionForSuggestion', [
  {
    name: 'leading dot completes next segment only',
    query: '.spec.tem',
    suggestion: 'spec.template.spec.containers[].image',
    want: '.spec.template',
  },
  {
    name: 'dotted path completes next segment only',
    query: 'spec.template.sp',
    suggestion: 'spec.template.spec.containers[].image',
    want: 'spec.template.spec',
  },
  {
    name: 'exact object path completes to object boundary first',
    query: '.spec.template',
    suggestion: 'spec.template.spec.containers[].image',
    want: '.spec.template.',
  },
  {
    name: 'non path query has no completion',
    query: 'helm',
    suggestion: 'spec.template.spec.containers[].image',
    want: '',
  },
  {
    name: 'mismatched suggestion has no completion',
    query: '.status.co',
    suggestion: 'spec.template.spec.containers[].image',
    want: '',
  },
  {
    name: 'completion is case insensitive for camel case fields',
    query: '.spec.volumeclaimt',
    suggestion: 'spec.volumeClaimTemplates',
    want: '.spec.volumeClaimTemplates',
  },
], ({ query, suggestion, want }) => {
  assert.equal(schemaSearch.completionForSuggestion(query, suggestion), want);
});

table('ghostSuffixForCompletion', [
  {
    name: 'partial segment shows only remaining suffix',
    query: '.spec.tem',
    completion: '.spec.template',
    want: 'plate',
  },
  {
    name: 'completed segment shows only next segment addition',
    query: '.spec.template',
    completion: '.spec.template.spec',
    want: '.spec',
  },
  {
    name: 'non path completion has no ghost suffix',
    query: 'helm',
    completion: '',
    want: '',
  },
  {
    name: 'ghost suffix is case insensitive for camel case completion',
    query: '.spec.volumeclaimt',
    completion: '.spec.volumeClaimTemplates',
    want: 'ClaimTemplates',
  },
], ({ query, completion, want }) => {
  assert.equal(schemaSearch.ghostSuffixForCompletion(query, completion), want);
});

table('ghostPrefixForCompletion', [
  {
    name: 'matching case keeps full typed prefix',
    query: '.spec.tem',
    completion: '.spec.template',
    want: '.spec.tem',
  },
  {
    name: 'camel case completion falls back to exact shared prefix',
    query: '.spec.volumeclaimt',
    completion: '.spec.volumeClaimTemplates',
    want: '.spec.volume',
  },
], ({ query, completion, want }) => {
  assert.equal(schemaSearch.ghostPrefixForCompletion(query, completion), want);
});

table('bestCompletionForPaths', [
  {
    name: 'exact object path prefers object boundary',
    query: '.spec.apiKeyAuth',
    suggestions: ['spec.apiKeyAuth.credentialRefs'],
    want: '.spec.apiKeyAuth.',
  },
  {
    name: 'completed segment with multiple next segments shows first ranked completion',
    query: '.spec.',
    suggestions: [
      'spec.template.spec.containers[].image',
      'spec.strategy.type',
    ],
    want: '.spec.strategy',
  },
  {
    name: 'completed segment with one next segment completes it',
    query: '.spec.',
    suggestions: [
      'spec.template.spec.containers[].image',
      'spec.template.metadata.labels',
    ],
    want: '.spec.template',
  },
  {
    name: 'partial segment still completes best match',
    query: '.spec.tem',
    suggestions: [
      'spec.template.spec.containers[].image',
      'spec.template.metadata.labels',
    ],
    want: '.spec.template',
  },
  {
    name: 'exact lowercase query still prefers camel case boundary',
    query: '.spec.volumeclaimtemplates',
    suggestions: ['spec.volumeClaimTemplates.name'],
    want: '.spec.volumeClaimTemplates.',
  },
], ({ query, suggestions, want }) => {
  assert.equal(schemaSearch.bestCompletionForPaths(query, suggestions), want);
});

table('completionCandidatesForPaths', [
  {
    name: 'exact object path exposes object boundary candidate',
    query: '.spec.apiKeyAuth',
    suggestions: ['spec.apiKeyAuth.credentialRefs'],
    want: ['.spec.apiKeyAuth.'],
  },
  {
    name: 'completed segment returns multiple unique next segments',
    query: '.spec.',
    suggestions: [
      'spec.template.spec.containers[].image',
      'spec.strategy.type',
      'spec.template.metadata.labels',
    ],
    want: ['.spec.strategy', '.spec.template'],
  },
  {
    name: 'partial segment deduplicates to one completion',
    query: '.spec.tem',
    suggestions: [
      'spec.template.spec.containers[].image',
      'spec.template.metadata.labels',
    ],
    want: ['.spec.template'],
  },
  {
    name: 'non path query has no candidates',
    query: 'helm',
    suggestions: ['spec.template.spec.containers[].image'],
    want: [],
  },
], ({ query, suggestions, want }) => {
  assert.deepEqual(schemaSearch.completionCandidatesForPaths(query, suggestions), want);
});

const defaultSuggestions = [
  'spec.replicas',
  'spec.targetNamespace',
  'spec.template.metadata.labels',
  'spec.template.spec.containers[].image',
  'spec.ipam.pools.allocated[].cidrs',
  'spec.issuerRef.group',
];

table('dotAdvanceForPathSearch', [
  {
    name: 'empty query can start strict path mode',
    query: '',
    want: '.',
  },
  {
    name: 'exact segment with children appends dot',
    query: '.spec',
    want: '.spec.',
  },
  {
    name: 'dotted exact segment with children appends dot',
    query: 'spec.template',
    want: 'spec.template.',
  },
  {
    name: 'unique partial segment advances conservatively',
    query: '.spec.tem',
    want: '.spec.template.',
  },
  {
    name: 'ambiguous partial segment does nothing',
    query: '.spec.t',
    want: '',
  },
  {
    name: 'terminal leaf does nothing',
    query: '.spec.issuerRef.group',
    want: '',
  },
  {
    name: 'array field advances to array item boundary',
    query: '.spec.ipam.pools.allocated',
    want: '.spec.ipam.pools.allocated[].',
  },
  {
    name: 'ambiguous trailing dot advances to first ranked child',
    query: '.spec.',
    want: '.spec.ipam',
  },
  {
    name: 'unique trailing dot advances to only child',
    query: '.spec.apiKeyAuth.',
    suggestions: ['spec.apiKeyAuth.credentialRefs'],
    want: '.spec.apiKeyAuth.credentialRefs',
  },
  {
    name: 'non path text cannot introduce invalid dot',
    query: 'helm',
    want: '',
  },
], ({ query, suggestions = defaultSuggestions, want }) => {
  assert.equal(schemaSearch.dotAdvanceForPathSearch(query, suggestions), want);
});

table('pathHasChildren', [
  {
    name: 'plain object field detects dotted children',
    query: '.spec.template',
    want: true,
  },
  {
    name: 'array field detects synthetic array item children',
    query: '.spec.ipam.pools.allocated',
    want: true,
  },
  {
    name: 'terminal child reports no children',
    query: '.spec.ipam.pools.allocated[].cidrs',
    want: false,
  },
], ({ query, want }) => {
  assert.equal(schemaSearch.pathHasChildren(query, [
    'spec.ipam.pools.allocated[].cidrs',
    'spec.template.spec.containers[].image',
  ]), want);
});

table('isPathLikeQuery', [
  {
    name: 'leading dot is path-like',
    query: '.spec',
    want: true,
  },
  {
    name: 'dotted partial path with completion is path-like',
    query: 'spec.tem',
    want: true,
  },
  {
    name: 'exact dotted leaf path is path-like',
    query: 'spec.issuerRef.group',
    want: true,
  },
  {
    name: 'virtual array item path is path-like',
    query: 'spec.ipam.pools.allocated[]',
    want: true,
  },
  {
    name: 'plain dotted text is not path-like',
    query: 'v1.2',
    want: false,
  },
  {
    name: 'plain text is not path-like',
    query: 'helm',
    want: false,
  },
], ({ query, want }) => {
  assert.equal(schemaSearch.isPathLikeQuery(query, [
    'spec.template.spec.containers[].image',
    'spec.ipam.pools.allocated[].cidrs',
    'spec.issuerRef.group',
  ]), want);
});

table('trimPathSearch', [
  {
    name: 'leading-dot child path trims to parent object boundary',
    query: '.spec.template',
    want: '.spec.',
  },
  {
    name: 'dotted child path trims to parent object boundary',
    query: 'spec.template.spec',
    want: 'spec.template.',
  },
  {
    name: 'leading-dot deep child path trims to immediate parent boundary',
    query: '.spec.rclone.customCA.configMapName',
    want: '.spec.rclone.customCA.',
  },
  {
    name: 'dotted deep child path trims to immediate parent boundary',
    query: 'spec.rclone.customCA.configMapName',
    want: 'spec.rclone.customCA.',
  },
  {
    name: 'leading-dot trailing boundary trims only trailing dot',
    query: '.spec.template.',
    want: '.spec.template',
  },
  {
    name: 'dotted trailing boundary trims only trailing dot',
    query: 'spec.template.',
    want: 'spec.template',
  },
  {
    name: 'single leading-dot segment clears',
    query: '.spec',
    want: '',
  },
  {
    name: 'single dotted segment clears',
    query: 'spec',
    want: '',
  },
  {
    name: 'plain text stays unchanged',
    query: 'helm-values',
    want: 'helm-values',
  },
], ({ query, want }) => {
  assert.equal(schemaSearch.trimPathSearch(query), want);
});
