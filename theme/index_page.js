(function(){
  var input = document.getElementById('search');
  var groups = document.querySelectorAll('.group');
  var noResults = document.getElementById('no-results');
  var statGroups = document.getElementById('stat-groups');
  var statSchemas = document.getElementById('stat-schemas');
  var totalGroups = groups.length;
  var totalSchemas = document.querySelectorAll('.schemas a').length;

  input.addEventListener('input', function(){
    var q = this.value.toLowerCase().trim();
    var visible = 0;
    groups.forEach(function(g){
      if (!q) {
        g.style.display = '';
        g.removeAttribute('open');
        g.querySelectorAll('.schemas a').forEach(function(a){ a.style.display = ''; });
        visible++;
        return;
      }
      var groupName = g.dataset.group.toLowerCase();
      var links = g.querySelectorAll('.schemas a');
      var groupMatch = groupName.indexOf(q) !== -1;
      var schemaMatch = false;
      links.forEach(function(a){
        if (a.dataset.schema.toLowerCase().indexOf(q) !== -1) {
          a.style.display = '';
          schemaMatch = true;
        } else {
          a.style.display = groupMatch ? '' : 'none';
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
      var visibleSchemas = 0;
      groups.forEach(function(g){
        if (g.style.display === 'none') return;
        g.querySelectorAll('.schemas a').forEach(function(a){
          if (a.style.display !== 'none') visibleSchemas++;
        });
      });
      statGroups.textContent = visible + ' / ' + totalGroups;
      statSchemas.textContent = visibleSchemas + ' / ' + totalSchemas;
    }
    writeHashSearchQuery(q);
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
          window.scrollTo({top: 0, behavior: 'smooth'});
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

  var toast = document.getElementById('toast');
  var toastTimer;
  document.getElementById('groups').addEventListener('click', function(e){
    if (!e.target.classList.contains('copy-hint')) return;
    e.preventDefault();
    var link = e.target.closest('.schemas a');
    var url = location.origin + link.dataset.url;
    navigator.clipboard.writeText(url).then(function(){
      clearTimeout(toastTimer);
      toast.classList.add('show');
      toastTimer = setTimeout(function(){ toast.classList.remove('show'); }, 1500);
    });
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
    var query = readHashSearchQuery();
    if (!query) return;
    input.value = query;
    input.dispatchEvent(new Event('input'));
  })();

  var btt = document.getElementById('back-to-top');
  window.addEventListener('scroll', function(){
    btt.classList.toggle('visible', window.scrollY > 300);
  }, {passive: true});
  btt.addEventListener('click', function(){
    window.scrollTo({top: 0, behavior: 'smooth'});
  });

  document.querySelectorAll('.usage-content code').forEach(function(el){
    var base = document.body.dataset.basePath || '';
    el.textContent = el.textContent.replace(/https:\/\/YOUR_DOMAIN/g, location.origin + base);
  });

  // Save view state before navigating to a schema page
  document.getElementById('groups').addEventListener('click', function(e){
    var link = e.target.closest('.schemas a');
    if (!link || e.target.classList.contains('copy-hint')) return;
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
