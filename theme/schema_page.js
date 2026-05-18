(function(){
  var input = document.getElementById('search');
  var searchClear = document.getElementById('search-clear');
  var searchGhost = document.getElementById('search-ghost');
  var searchGhostPrefix = document.getElementById('search-ghost-prefix');
  var searchGhostSuffix = document.getElementById('search-ghost-suffix');
  var searchStatus = document.getElementById('search-status');
  var noResults = document.getElementById('no-results');
  var props = document.querySelectorAll('.prop');
  var rows = Array.prototype.slice.call(document.querySelectorAll('[data-prop-row]'));
  var learnedPathSearchStorageKey = 'crd-schema-publisher:path-search-learned';
  var completionCandidates = [];
  var completionIndex = -1;
  var rowByPath = {};
  rows.forEach(function(row){
    rowByPath[row.dataset.path] = row;
  });

  function visibleRows() {
    return rows.filter(function(row){ return row.style.display !== 'none'; });
  }

  function setSearchStatus(message, hasResults) {
    searchStatus.textContent = message;
    searchStatus.classList.toggle('has-results', !!hasResults);
  }

  function updateSearchClear() {
    searchClear.hidden = !input.value;
  }

  function hasLearnedPathSearch() {
    try {
      return localStorage.getItem(learnedPathSearchStorageKey) === '1';
    } catch (err) {
      return false;
    }
  }

  function markPathSearchLearned(rawQuery) {
    var query = (rawQuery || '').trim();
    if (query.indexOf('.') !== 0) {
      return;
    }
    var queryState = splitPathSegments(query);
    if (queryState.segments.length < 2) {
      return;
    }
    try {
      localStorage.setItem(learnedPathSearchStorageKey, '1');
    } catch (err) {
      // Ignore storage failures and fall back to showing the tip.
    }
  }

  function currentSuggestions() {
    return rows.map(function(row){ return row.dataset.path || ''; });
  }

  function selectedCompletion() {
    if (completionIndex < 0 || completionIndex >= completionCandidates.length) {
      return '';
    }
    return completionCandidates[completionIndex];
  }

  function bestCompletionForQuery(query) {
    return bestCompletionForPaths(query, currentSuggestions());
  }

  function updateGhostSuggestion(rawQuery) {
    var completion = selectedCompletion() || bestCompletionForQuery(rawQuery);
    var suffix = ghostSuffixForCompletion(input.value, completion);
    var prefix = ghostPrefixForCompletion(input.value, completion);
    searchGhost.hidden = !suffix;
    searchGhostPrefix.textContent = suffix ? prefix : '';
    searchGhostSuffix.textContent = suffix;
  }

  function clearSearchState() {
    rows.forEach(function(row){
      row.style.display = '';
      row.classList.remove('search-match', 'search-ancestor');
      if (row.tagName === 'DETAILS') {
        row.removeAttribute('open');
      }
    });
    noResults.style.display = 'none';
    setSearchStatus(hasLearnedPathSearch() ? '' : (searchStatus.dataset.emptyMessage || ''), false);
    searchGhost.hidden = true;
    searchGhostPrefix.textContent = '';
    searchGhostSuffix.textContent = '';
    completionCandidates = [];
    completionIndex = -1;
    updateSearchClear();
  }

  document.getElementById('copy-url').addEventListener('click', function(){
    copyURLWithToast(location.origin + this.dataset.url);
  });

  var schemaSearch = window.SchemaSearch;
  if (!schemaSearch) {
    clearSearchState();
    input.value = '';
    input.disabled = true;
    input.placeholder = 'Schema search unavailable';
    setSearchStatus('Search unavailable', false);
    if (window.console && console.warn) {
      console.warn('schema-search.js failed to load; schema search disabled');
    }
    document.getElementById('expand-all').addEventListener('click', function(){
      props.forEach(function(p){ p.setAttribute('open',''); });
    });
    document.getElementById('collapse-all').addEventListener('click', function(){
      props.forEach(function(p){ p.removeAttribute('open'); });
    });
    document.addEventListener('keydown', function(e){
      if (e.key !== 'Escape') {
        return;
      }
      e.preventDefault();
      var hadOpen = false;
      visibleRows().forEach(function(row){
        if (row.tagName === 'DETAILS' && row.hasAttribute('open')) {
          hadOpen = true;
          row.removeAttribute('open');
        }
      });
      if (hadOpen) {
        return;
      }
      if (document.activeElement && document.activeElement !== document.body) {
        document.activeElement.blur();
        return;
      }
      location.href = document.body.dataset.basePath + '/';
    });
    return;
  }

  var bestCompletionForPaths = schemaSearch.bestCompletionForPaths;
  var completionCandidatesForPaths = schemaSearch.completionCandidatesForPaths;
  var ghostPrefixForCompletion = schemaSearch.ghostPrefixForCompletion;
  var ghostSuffixForCompletion = schemaSearch.ghostSuffixForCompletion;
  var dotAdvanceForPathSearch = schemaSearch.dotAdvanceForPathSearch;
  var isPathLikeQuery = schemaSearch.isPathLikeQuery;
  var matchesPathQuery = schemaSearch.matchesPathQuery;
  var splitPathSegments = schemaSearch.splitPathSegments;
  var trimPathSearch = schemaSearch.trimPathSearch;

  function addAncestorPaths(path, visiblePaths, openPaths) {
    var current = path;
    var directMatch = true;
    while (current) {
      visiblePaths[current] = true;
      if (!directMatch) {
        openPaths[current] = true;
      }
      var row = rowByPath[current];
      if (!row || !row.dataset.parentPath) {
        break;
      }
      current = row.dataset.parentPath;
      directMatch = false;
    }
  }

  function applySearch(rawQuery) {
    updateSearchClear();
    var trimmedQuery = rawQuery.trim();
    var query = trimmedQuery.toLowerCase();
    if (!query) {
      clearSearchState();
      writeHashSearchQuery('');
      return;
    }

    var pathOnly = query.indexOf('.') === 0;
    if (pathOnly) {
      query = query.slice(1);
    }
    if (pathOnly && !query) {
      clearSearchState();
      writeHashSearchQuery('');
      return;
    }

    markPathSearchLearned(rawQuery);

    completionCandidates = completionCandidatesForPaths(rawQuery.trim(), currentSuggestions());
    completionIndex = -1;

    var directMatches = {};
    var visiblePaths = {};
    var openPaths = {};
    rows.forEach(function(row){
      var path = (row.dataset.path || '').toLowerCase();
      var name = (row.dataset.name || '').toLowerCase();
      var text = (row.dataset.text || '').toLowerCase();
      var pathMatch = query.indexOf('.') !== -1
        ? matchesPathQuery(path, trimmedQuery)
        : path.indexOf(query) !== -1;
      var matched = pathOnly
        ? matchesPathQuery(path, trimmedQuery)
        : pathMatch || name.indexOf(query) !== -1 || text.indexOf(query) !== -1;
      if (!matched) {
        return;
      }
      directMatches[row.dataset.path] = true;
      addAncestorPaths(row.dataset.path, visiblePaths, openPaths);
    });

    var selectedRow = rows.find(function(row){ return !!directMatches[row.dataset.path]; });
    if (selectedRow && selectedRow.tagName === 'DETAILS') {
      openPaths[selectedRow.dataset.path] = true;
    }

    var matchCount = Object.keys(directMatches).length;
    rows.forEach(function(row){
      var path = row.dataset.path;
      var visible = !!visiblePaths[path];
      var open = !!openPaths[path];
      row.style.display = visible ? '' : 'none';
      row.classList.toggle('search-match', !!directMatches[path]);
      row.classList.toggle('search-ancestor', visible && !directMatches[path]);
      if (row.tagName === 'DETAILS') {
        if (open) {
          row.setAttribute('open', '');
        } else {
          row.removeAttribute('open');
        }
      }
    });

    noResults.style.display = matchCount ? 'none' : 'block';
    noResults.textContent = noResults.dataset.noResultsMessage || 'No matches';
    setSearchStatus(matchCount ? matchCount + ' matches' : 'No matches', matchCount > 0);
    updateGhostSuggestion(rawQuery.trim());
    writeHashSearchQuery(rawQuery.trim());
  }

  document.getElementById('expand-all').addEventListener('click', function(){
    if (input.value) {
      visibleRows().forEach(function(row){
        if (row.tagName === 'DETAILS') row.setAttribute('open', '');
      });
      return;
    }
    props.forEach(function(p){ p.setAttribute('open',''); });
  });
  document.getElementById('collapse-all').addEventListener('click', function(){
    if (input.value) {
      applySearch(input.value);
      return;
    }
    props.forEach(function(p){ p.removeAttribute('open'); });
  });
  input.addEventListener('input', function(){
    applySearch(this.value);
  });
  searchClear.addEventListener('mousedown', function(e){
    e.preventDefault();
  });
  searchClear.addEventListener('click', function(){
    input.value = '';
    applySearch('');
    input.focus();
  });
  document.addEventListener('keydown', function(e){
    if (e.key === 'ArrowDown' && document.activeElement === input) {
      if (completionCandidates.length) {
        e.preventDefault();
        completionIndex = completionIndex < 0 ? 0 : (completionIndex + 1) % completionCandidates.length;
        updateGhostSuggestion(input.value);
      }
      return;
    }
    if (e.key === 'ArrowUp' && document.activeElement === input) {
      if (completionCandidates.length) {
        e.preventDefault();
        completionIndex = completionIndex < 0 ? completionCandidates.length - 1 : (completionIndex - 1 + completionCandidates.length) % completionCandidates.length;
        updateGhostSuggestion(input.value);
      }
      return;
    }
    if ((e.key === 'Tab' || e.key === 'ArrowRight') && document.activeElement === input) {
      var caretAtEnd = input.selectionStart === input.value.length && input.selectionEnd === input.value.length;
      if (!caretAtEnd) {
        return;
      }
      var completion = selectedCompletion() || bestCompletionForQuery(input.value);
      if (completion) {
        e.preventDefault();
        input.value = completion;
        applySearch(completion);
      }
      return;
    }
    if (e.key === '.' && document.activeElement === input && !e.ctrlKey && !e.metaKey && !e.altKey) {
      var pathLikeQuery = isPathLikeQuery(input.value, currentSuggestions());
      if (pathLikeQuery) {
        if (input.selectionStart === input.value.length && input.selectionEnd === input.value.length) {
          e.preventDefault();
          var dotAdvance = dotAdvanceForPathSearch(input.value, currentSuggestions());
          if (dotAdvance) {
            input.value = dotAdvance;
            applySearch(dotAdvance);
          }
        }
        return;
      }
    }
    if (e.key === '/' && !e.ctrlKey && !e.metaKey && document.activeElement !== input) {
      e.preventDefault();
      input.focus();
      return;
    }
    if (e.key === 'Escape') {
      e.preventDefault();
      if (input.value) {
        var trimmed = trimPathSearch(input.value);
        if (trimmed !== input.value) {
          input.value = trimmed;
          applySearch(trimmed);
        } else {
          input.value = '';
          applySearch('');
          input.blur();
        }
        return;
      }
      var hadOpen = false;
      visibleRows().forEach(function(row){
        if (row.tagName === 'DETAILS' && row.hasAttribute('open')) {
          hadOpen = true;
          row.removeAttribute('open');
        }
      });
      if (hadOpen) {
        return;
      }
      if (document.activeElement && document.activeElement !== document.body) {
        document.activeElement.blur();
        return;
      }
      location.href = document.body.dataset.basePath + '/';
    }
  });
  (function(){
    clearSearchState();
    var query = readHashSearchQuery();
    if (!query) return;
    input.value = query;
    applySearch(query);
  })();
})();
