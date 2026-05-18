(function(){
  var input = document.getElementById('search');
  var searchClear = document.getElementById('search-clear');
  var groups = document.querySelectorAll('.group');
  var noResults = document.getElementById('no-results');
  var searchStatus = document.getElementById('search-status');
  var statGroups = document.getElementById('stat-groups');
  var statSchemas = document.getElementById('stat-schemas');
  var totalGroups = groups.length;
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
      searchStatus.textContent = 'No matching groups or schemas.';
      return;
    }
    searchStatus.textContent = visibleGroups + ' of ' + totalGroups + ' API groups, ' + visibleSchemas + ' of ' + totalSchemas + ' schemas shown.';
  }

  input.addEventListener('input', function(){
    var q = this.value.toLowerCase().trim();
    updateSearchClear();
    var visible = 0;
    var visibleSchemas = 0;
    groups.forEach(function(g){
      var rows = g.querySelectorAll('.schema-row');
      if (!q) {
        g.style.display = '';
        g.removeAttribute('open');
        rows.forEach(function(row){ row.style.display = ''; });
        visible++;
        visibleSchemas += rows.length;
        return;
      }
      var groupName = g.dataset.group.toLowerCase();
      var groupMatch = groupName.indexOf(q) !== -1;
      var schemaMatch = false;
      rows.forEach(function(row){
        if (row.dataset.schema.toLowerCase().indexOf(q) !== -1) {
          row.style.display = '';
          schemaMatch = true;
        } else {
          row.style.display = groupMatch ? '' : 'none';
        }
        if (row.style.display !== 'none') {
          visibleSchemas++;
        }
      });
      if (groupMatch || schemaMatch) {
        g.style.display = '';
        g.setAttribute('open','');
        visible++;
      } else {
        g.style.display = 'none';
        g.removeAttribute('open');
      }
    });
    noResults.style.display = visible ? 'none' : 'block';
    if (!q) {
      statGroups.textContent = totalGroups;
      statSchemas.textContent = totalSchemas;
    } else {
      statGroups.textContent = visible + ' / ' + totalGroups;
      statSchemas.textContent = visibleSchemas + ' / ' + totalSchemas;
    }
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

  var toggleAll = document.getElementById('toggle-all');
  var allExpanded = false;
  toggleAll.addEventListener('click', function(){
    allExpanded = !allExpanded;
    groups.forEach(function(g){
      if (g.style.display === 'none') return;
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
    var expanded = [];
    groups.forEach(function(g){ if (g.hasAttribute('open')) expanded.push(g.dataset.group); });
    sessionStorage.setItem('indexState', JSON.stringify({
      expanded: expanded,
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
      if (state.expanded && state.expanded.length) {
        groups.forEach(function(g){
          if (state.expanded.indexOf(g.dataset.group) !== -1) g.setAttribute('open','');
        });
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
