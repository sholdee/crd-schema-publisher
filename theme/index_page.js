(function(){
  var input = document.getElementById('search');
  var searchClear = document.getElementById('search-clear');
  var sourceSections = document.querySelectorAll('.source-section');
  var groups = document.querySelectorAll('.group');
  var noResults = document.getElementById('no-results');
  var searchStatus = document.getElementById('search-status');
  var statGroups = document.getElementById('stat-groups');
  var statSchemas = document.getElementById('stat-schemas');
  var toggleAll = document.getElementById('toggle-all');
  var allExpanded = false;
  var totalGroups = countUniqueGroups(groups);
  var totalSchemas = document.querySelectorAll('.schema-row').length;

  function updateSearchClear() {
    searchClear.hidden = !input.value;
  }

  function updateSearchStatus(q, visibleGroups, visibleSchemas) {
    if (!q) {
      searchStatus.textContent = '';
      return;
    }
    if (!visibleGroups) {
      searchStatus.textContent = 'No matching sources, groups, or schemas.';
      return;
    }
    searchStatus.textContent = visibleGroups + ' of ' + totalGroups + ' API groups, ' + visibleSchemas + ' of ' + totalSchemas + ' schemas shown.';
  }

  function isVisible(el) {
    return el.style.display !== 'none';
  }

  function setOpen(el, open) {
    if (open) el.setAttribute('open','');
    else el.removeAttribute('open');
  }

  function sourceDefaultOpen(source) {
    return source.dataset.defaultOpen === 'true';
  }

  function groupKey(group) {
    return (group.dataset.source || 'crd') + ':' + group.dataset.group;
  }

  function countUniqueGroups(groupList) {
    var names = {};
    groupList.forEach(function(group){
      names[group.dataset.group] = true;
    });
    return Object.keys(names).length;
  }

  function restoreDefaultOpenState() {
    sourceSections.forEach(function(source){
      source.style.display = '';
      setOpen(source, sourceDefaultOpen(source));
    });
    groups.forEach(function(group){
      group.style.display = '';
      group.removeAttribute('open');
      group.querySelectorAll('.schema-row').forEach(function(row){ row.style.display = ''; });
    });
    noResults.style.display = 'none';
    statGroups.textContent = totalGroups;
    statSchemas.textContent = totalSchemas;
    updateSearchStatus('', totalGroups, totalSchemas);
    allExpanded = false;
    toggleAll.textContent = 'Expand all';
  }

  function applyGroupSearch(g, q, sourceMatch) {
    var rows = g.querySelectorAll('.schema-row');
    var groupName = g.dataset.group.toLowerCase();
    var groupMatch = groupName.indexOf(q) !== -1;
    var schemaMatch = false;
    var shownRows = 0;
    rows.forEach(function(row){
      var rowMatch = row.dataset.schema.toLowerCase().indexOf(q) !== -1;
      var showRow = sourceMatch || groupMatch || rowMatch;
      row.style.display = showRow ? '' : 'none';
      if (rowMatch) schemaMatch = true;
      if (showRow) shownRows++;
    });
    if (sourceMatch || groupMatch || schemaMatch) {
      g.style.display = '';
      g.setAttribute('open','');
      return shownRows;
    }
    g.style.display = 'none';
    g.removeAttribute('open');
    return 0;
  }

  input.addEventListener('input', function(){
    var q = this.value.toLowerCase().trim();
    updateSearchClear();
    if (!q) {
      restoreDefaultOpenState();
      writeHashSearchQuery(q);
      return;
    }

    var visibleGroupsByName = {};
    var visibleSchemas = 0;
    if (sourceSections.length) {
      sourceSections.forEach(function(source){
        var sourceKey = (source.dataset.source || '').toLowerCase();
        var sourceLabel = (source.dataset.sourceLabel || '').toLowerCase();
        var sourceMatch = sourceKey.indexOf(q) !== -1 || sourceLabel.indexOf(q) !== -1;
        var sourceVisible = false;
        source.querySelectorAll('.group').forEach(function(g){
          var shownRows = applyGroupSearch(g, q, sourceMatch);
          if (shownRows) {
            sourceVisible = true;
            visibleGroupsByName[g.dataset.group] = true;
            visibleSchemas += shownRows;
          }
        });
        if (sourceVisible) {
          source.style.display = '';
          source.setAttribute('open','');
        } else {
          source.style.display = 'none';
          source.removeAttribute('open');
        }
      });
    } else {
      groups.forEach(function(g){
        var shownRows = applyGroupSearch(g, q, false);
        if (shownRows) {
          visibleGroupsByName[g.dataset.group] = true;
          visibleSchemas += shownRows;
        }
      });
    }
    var visible = Object.keys(visibleGroupsByName).length;
    noResults.style.display = visible ? 'none' : 'block';
    statGroups.textContent = visible + ' / ' + totalGroups;
    statSchemas.textContent = visibleSchemas + ' / ' + totalSchemas;
    updateSearchStatus(q, visible, visibleSchemas);
    writeHashSearchQuery(q);
  });

  searchClear.addEventListener('mousedown', function(e){
    e.preventDefault();
  });
  searchClear.addEventListener('click', function(){
    input.value = '';
    input.dispatchEvent(new Event('input'));
    input.focus();
  });

  document.addEventListener('keydown', function(e){
    if (e.key === 'Escape') {
      e.preventDefault();
      if (input.value) {
        input.value = '';
        input.dispatchEvent(new Event('input'));
      } else if (document.activeElement === input) {
        input.blur();
      } else {
        var hadOpen = false;
        groups.forEach(function(g){ if (g.hasAttribute('open')) { hadOpen = true; g.removeAttribute('open'); } });
        sourceSections.forEach(function(source){
          if (!sourceDefaultOpen(source) && source.hasAttribute('open')) {
            hadOpen = true;
            source.removeAttribute('open');
          }
        });
        if (hadOpen) {
          allExpanded = false;
          toggleAll.textContent = 'Expand all';
        } else {
          scrollToTop();
        }
        if (document.activeElement && document.activeElement !== document.body) {
          document.activeElement.blur();
        }
      }
    }
    if (e.key === '/' && !e.ctrlKey && !e.metaKey && document.activeElement !== input) {
      e.preventDefault();
      input.focus();
    }
  });

  document.getElementById('groups').addEventListener('click', function(e){
    var copyButton = e.target.closest('.schema-copy');
    if (!copyButton) return;
    e.preventDefault();
    copyURLWithToast(location.origin + copyButton.dataset.url);
  });

  toggleAll.addEventListener('click', function(){
    allExpanded = !allExpanded;
    sourceSections.forEach(function(source){
      if (!isVisible(source)) return;
      setOpen(source, allExpanded || sourceDefaultOpen(source));
    });
    groups.forEach(function(g){
      if (!isVisible(g)) return;
      if (allExpanded) g.setAttribute('open','');
      else g.removeAttribute('open');
    });
    toggleAll.textContent = allExpanded ? 'Collapse all' : 'Expand all';
  });

  (function(){
    updateSearchClear();
    var query = readHashSearchQuery();
    if (!query) return;
    input.value = query;
    input.dispatchEvent(new Event('input'));
  })();

  document.querySelectorAll('.usage-content code').forEach(function(el){
    var base = document.body.dataset.basePath || '';
    el.textContent = el.textContent.replace(/https:\/\/YOUR_DOMAIN/g, location.origin + base);
  });

  // Save view state before navigating to a schema page
  document.getElementById('groups').addEventListener('click', function(e){
    if (e.target.closest('.schema-copy')) return;
    var link = e.target.closest('.schema-row a');
    if (!link) return;
    var expandedSources = [];
    var expandedGroups = [];
    sourceSections.forEach(function(source){ if (source.hasAttribute('open')) expandedSources.push(source.dataset.source); });
    groups.forEach(function(g){ if (g.hasAttribute('open')) expandedGroups.push(groupKey(g)); });
    sessionStorage.setItem('indexState', JSON.stringify({
      sources: expandedSources,
      groups: expandedGroups,
      scroll: window.scrollY,
      toggleAll: allExpanded,
      search: input.value
    }));
  });

  // Restore view state when returning from a schema page
  var saved = sessionStorage.getItem('indexState');
  if (!hasHashSearchQuery() && saved) {
    sessionStorage.removeItem('indexState');
    try {
      var state = JSON.parse(saved);
      if (state.search) {
        input.value = state.search;
        input.dispatchEvent(new Event('input'));
      }
      var savedSources = Array.isArray(state.sources) ? state.sources : state.expandedSources;
      var savedGroups = Array.isArray(state.groups) ? state.groups : state.expandedGroups;
      if (!Array.isArray(savedGroups) && Array.isArray(state.expanded)) {
        savedGroups = state.expanded.map(function(name){
          return name.indexOf(':') === -1 ? 'crd:' + name : name;
        });
      }
      if (Array.isArray(savedSources) || Array.isArray(savedGroups)) {
        var openSources = {};
        var openGroups = {};
        if (Array.isArray(savedSources)) {
          savedSources.forEach(function(source){ openSources[source] = true; });
        }
        if (Array.isArray(savedGroups)) {
          savedGroups.forEach(function(key){
            openGroups[key] = true;
            openSources[key.split(':')[0]] = true;
          });
        }
        sourceSections.forEach(function(source){ setOpen(source, !!openSources[source.dataset.source]); });
        groups.forEach(function(g){ setOpen(g, !!openGroups[groupKey(g)]); });
      }
      if (state.toggleAll) {
        allExpanded = true;
        toggleAll.textContent = 'Collapse all';
      }
      if (state.scroll) {
        window.scrollTo(0, state.scroll);
      }
    } catch(e) {}
  }
})();
