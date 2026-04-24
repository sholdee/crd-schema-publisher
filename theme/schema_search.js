(function(root, factory) {
  if (typeof module === 'object' && module.exports) {
    module.exports = factory();
    return;
  }
  root.SchemaSearch = factory();
})(typeof globalThis !== 'undefined' ? globalThis : this, function() {
  'use strict';

  function splitPathSegments(path) {
    path = (path || '').trim().replace(/^\./, '');
    if (!path) {
      return { segments: [], trailingDot: false };
    }

    return {
      segments: path.split('.').filter(Boolean),
      trailingDot: path.lastIndexOf('.') === path.length - 1
    };
  }

  function matchesPathQuery(path, rawQuery) {
    var pathState = splitPathSegments(path);
    var queryState = splitPathSegments(rawQuery);
    if (!pathState.segments.length || !queryState.segments.length) {
      return false;
    }

    if (!queryState.trailingDot &&
        /\[\]$/.test(queryState.segments[queryState.segments.length - 1]) &&
        pathState.segments.length === queryState.segments.length + 1) {
      for (var j = 0; j < queryState.segments.length; j++) {
        if (pathState.segments[j].toLowerCase() !== queryState.segments[j].toLowerCase()) {
          return false;
        }
      }
      return true;
    }

    var expectedLength = queryState.segments.length + (queryState.trailingDot ? 1 : 0);
    if (pathState.segments.length !== expectedLength) {
      return false;
    }

    for (var i = 0; i < queryState.segments.length - 1; i++) {
      if (pathState.segments[i].toLowerCase() !== queryState.segments[i].toLowerCase()) {
        return false;
      }
    }

    var lastIndex = queryState.segments.length - 1;
    if (queryState.trailingDot) {
      return pathState.segments[lastIndex].toLowerCase() === queryState.segments[lastIndex].toLowerCase();
    }
    return pathState.segments[lastIndex].toLowerCase().indexOf(queryState.segments[lastIndex].toLowerCase()) === 0;
  }

  function completionForSuggestion(rawQuery, suggestion) {
    var query = (rawQuery || '').trim();
    suggestion = (suggestion || '').trim();
    if (!query || !suggestion) {
      return '';
    }

    var leadingDot = query.indexOf('.') === 0;
    var normalized = leadingDot ? query.slice(1) : query;
    if (!leadingDot && normalized.indexOf('.') === -1) {
      return '';
    }
    if (!normalized) {
      return '';
    }

    var queryParts = normalized.split('.');
    var suggestionParts = suggestion.split('.');
    if (suggestionParts.length < queryParts.length) {
      return '';
    }

    for (var i = 0; i < queryParts.length - 1; i++) {
      if (queryParts[i].toLowerCase() !== suggestionParts[i].toLowerCase()) {
        return '';
      }
    }

    var lastIndex = queryParts.length - 1;
    var lastQuery = queryParts[lastIndex];
    var lastSuggestion = suggestionParts[lastIndex];
    if (lastQuery.toLowerCase() === lastSuggestion.toLowerCase() && suggestionParts.length > queryParts.length) {
      var boundaryParts = queryParts.slice();
      boundaryParts[lastIndex] = lastSuggestion;
      var boundaryResult = boundaryParts.join('.') + '.';
      return leadingDot ? '.' + boundaryResult : boundaryResult;
    }
    if (lastSuggestion.toLowerCase().indexOf(lastQuery.toLowerCase()) !== 0) {
      return '';
    }

    var completed = queryParts.slice();
    if (lastQuery !== lastSuggestion) {
      completed[lastIndex] = lastSuggestion;
    } else if (suggestionParts.length > queryParts.length) {
      completed.push(suggestionParts[queryParts.length]);
    } else {
      return '';
    }

    var result = completed.join('.');
    return leadingDot ? '.' + result : result;
  }

  function completionCandidatesForPaths(rawQuery, suggestions) {
    var query = (rawQuery || '').trim();
    if (!query) {
      return [];
    }

    var seen = Object.create(null);
    var candidates = [];
    suggestions.forEach(function(suggestion) {
      var completion = completionForSuggestion(query, suggestion);
      if (!completion || seen[completion]) {
        return;
      }
      seen[completion] = true;
      candidates.push(completion);
    });
    candidates.sort();
    return candidates;
  }

  function ghostPrefixForCompletion(rawQuery, completion) {
    var query = (rawQuery || '').trim();
    completion = (completion || '').trim();
    if (!query || !completion) {
      return '';
    }
    if (completion.toLowerCase().indexOf(query.toLowerCase()) !== 0) {
      return '';
    }

    var max = Math.min(query.length, completion.length);
    var i = 0;
    while (i < max && completion.charAt(i) === query.charAt(i)) {
      i++;
    }
    if (i === query.length) {
      return query;
    }
    return completion.slice(0, i);
  }

  function ghostSuffixForCompletion(rawQuery, completion) {
    var query = (rawQuery || '').trim();
    completion = (completion || '').trim();
    if (!query || !completion) {
      return '';
    }
    if (completion.toLowerCase().indexOf(query.toLowerCase()) !== 0) {
      return '';
    }
    return completion.slice(ghostPrefixForCompletion(query, completion).length);
  }

  function bestCompletionForPaths(rawQuery, suggestions) {
    var query = (rawQuery || '').trim();
    if (!query) {
      return '';
    }

    var candidates = completionCandidatesForPaths(query, suggestions);
    if (!candidates.length) {
      return '';
    }
    return candidates[0];
  }

  function pathHasChildren(rawQuery, suggestions) {
    var query = (rawQuery || '').trim();
    if (!query) {
      return false;
    }

    var normalized = query.replace(/^\./, '');
    if (!normalized) {
      return false;
    }

    var lowerNormalized = normalized.toLowerCase();
    return suggestions.some(function(suggestion) {
      var lowerSuggestion = (suggestion || '').trim().toLowerCase();
      return lowerSuggestion.indexOf(lowerNormalized + '.') === 0 || lowerSuggestion.indexOf(lowerNormalized + '[].') === 0;
    });
  }

  function dotAdvanceForPathSearch(rawQuery, suggestions) {
    var query = (rawQuery || '').trim();
    if (!query) {
      return '.';
    }
    if (query.lastIndexOf('.') === query.length - 1) {
      return bestCompletionForPaths(query, suggestions);
    }

    var candidates = completionCandidatesForPaths(query, suggestions);
    if (candidates.length === 1 && candidates[0] !== query) {
      if (candidates[0].lastIndexOf('.') === candidates[0].length - 1) {
        return candidates[0];
      }
      if (!pathHasChildren(candidates[0], suggestions)) {
        return '';
      }
      return candidates[0] + '.';
    }
    if (pathHasChildren(query, suggestions)) {
      return query + '.';
    }
    return '';
  }

  function isPathLikeQuery(rawQuery, suggestions) {
    var query = (rawQuery || '').trim();
    if (!query) {
      return true;
    }
    if (query.indexOf('.') === 0) {
      return true;
    }
    if (query.indexOf('.') === -1) {
      return false;
    }
    if (completionCandidatesForPaths(query, suggestions).length) {
      return true;
    }

    var lowerQuery = query.toLowerCase();
    return suggestions.some(function(suggestion) {
      var lowerSuggestion = (suggestion || '').trim().toLowerCase();
      return lowerSuggestion === lowerQuery || matchesPathQuery(lowerSuggestion, query);
    });
  }

  function trimPathSearch(rawQuery) {
    var query = (rawQuery || '').trim();
    if (!query) {
      return '';
    }
    if (query.lastIndexOf('.') === query.length - 1) {
      var trimmedBoundary = query.slice(0, -1);
      return trimmedBoundary === '.' ? '' : trimmedBoundary;
    }

    var leadingDot = query.indexOf('.') === 0;
    var normalized = leadingDot ? query.slice(1) : query;
    if (normalized.indexOf('.') === -1) {
      if (leadingDot || !/[-_/ ]/.test(normalized)) {
        return '';
      }
      return query;
    }

    var parts = normalized.split('.').filter(Boolean);
    if (parts.length <= 1) {
      return '';
    }
    var result = parts.slice(0, -1).join('.') + '.';
    return leadingDot ? '.' + result : result;
  }

  return {
    bestCompletionForPaths: bestCompletionForPaths,
    completionCandidatesForPaths: completionCandidatesForPaths,
    completionForSuggestion: completionForSuggestion,
    dotAdvanceForPathSearch: dotAdvanceForPathSearch,
    ghostPrefixForCompletion: ghostPrefixForCompletion,
    ghostSuffixForCompletion: ghostSuffixForCompletion,
    isPathLikeQuery: isPathLikeQuery,
    matchesPathQuery: matchesPathQuery,
    pathHasChildren: pathHasChildren,
    splitPathSegments: splitPathSegments,
    trimPathSearch: trimPathSearch
  };
});
