document.addEventListener("DOMContentLoaded", () => {
  const searchForm = document.querySelector(".site-search");
  if (searchForm) {
    initSearch(searchForm);
  }

  const sidebar = document.querySelector(".site-sidebar");
  if (sidebar) {
    initSidebarPersistence(sidebar);
  }

  let copyButtonIndex = 0;
  document.querySelectorAll(".doc-content pre").forEach((block) => {
    copyButtonIndex += 1;
    const copyButtonNumber = copyButtonIndex;
    const button = document.createElement("button");
    button.type = "button";
    button.className = "copy-button";
    button.textContent = "Copy";
    button.setAttribute("aria-label", `Copy code block ${copyButtonNumber}`);
    button.addEventListener("click", async () => {
      const code = block.querySelector("code");
      if (!code || !navigator.clipboard) {
        return;
      }
      await navigator.clipboard.writeText(code.innerText);
      button.textContent = "Copied";
      button.setAttribute("aria-label", `Copied code block ${copyButtonNumber}`);
      window.setTimeout(() => {
        button.textContent = "Copy";
        button.setAttribute("aria-label", `Copy code block ${copyButtonNumber}`);
      }, 1500);
    });
    block.append(button);
  });

  document.querySelectorAll(".doc-content h2[id], .doc-content h3[id]").forEach((heading) => {
    const link = document.createElement("a");
    link.className = "heading-permalink";
    link.href = `#${heading.id}`;
    link.setAttribute("aria-label", `Link to ${heading.textContent}`);
    link.textContent = "#";
    heading.append(" ", link);
  });
});

function initSidebarPersistence(sidebar) {
  const storageKey = "crdsp.sidebar.scrollTop";
  const storedScrollTop = Number.parseInt(storageGet("sessionStorage", storageKey) || "", 10);
  if (Number.isFinite(storedScrollTop) && storedScrollTop > 0) {
    window.requestAnimationFrame(() => {
      sidebar.scrollTop = storedScrollTop;
    });
  }
  let scheduled = false;
  sidebar.addEventListener("scroll", () => {
    if (scheduled) {
      return;
    }
    scheduled = true;
    window.requestAnimationFrame(() => {
      scheduled = false;
      storageSet("sessionStorage", storageKey, String(Math.round(sidebar.scrollTop)));
    });
  }, { passive: true });
}

function initSearch(form) {
  const input = form.querySelector("input[type='search']");
  const panel = form.querySelector(".search-panel");
  const resultsList = form.querySelector("#site-search-results");
  const indexUrl = form.dataset.searchIndex;
  let pages = [];
  let activeIndex = -1;

  if (!input || !panel || !resultsList || !indexUrl) {
    return;
  }

  const loadIndex = async () => {
    if (pages.length > 0) {
      return pages;
    }
    const response = await fetch(indexUrl, { credentials: "same-origin" });
    if (!response.ok) {
      throw new Error(`Search index unavailable: ${response.status}`);
    }
    pages = await response.json();
    return pages;
  };

  const closeResults = () => {
    panel.hidden = true;
    input.setAttribute("aria-expanded", "false");
    input.removeAttribute("aria-activedescendant");
    activeIndex = -1;
  };

  const clearSearch = () => {
    input.value = "";
    resultsList.replaceChildren();
    closeResults();
  };

  const clearOrBlurSearch = () => {
    if (input.value) {
      clearSearch();
      return;
    }
    closeResults();
    if (document.activeElement === input) {
      input.blur();
    }
  };

  const setActive = (nextIndex) => {
    const items = Array.from(resultsList.querySelectorAll("[role='option']"));
    if (items.length === 0) {
      activeIndex = -1;
      input.removeAttribute("aria-activedescendant");
      return;
    }
    activeIndex = (nextIndex + items.length) % items.length;
    items.forEach((item, index) => {
      item.setAttribute("aria-selected", index === activeIndex ? "true" : "false");
    });
    input.setAttribute("aria-activedescendant", items[activeIndex].id);
  };

  const renderResults = (matches, terms) => {
    resultsList.replaceChildren();
    if (matches.length === 0) {
      renderStatusResult("No results");
      return;
    }
    matches.forEach((match, index) => {
      const page = match.page;
      const item = document.createElement("li");
      const link = document.createElement("a");
      const title = document.createElement("span");
      const snippet = document.createElement("small");
      item.id = `site-search-result-${index}`;
      item.setAttribute("role", "option");
      item.setAttribute("aria-selected", "false");
      link.href = page.url;
      appendHighlightedText(title, page.title || "", terms);
      appendHighlightedText(snippet, match.snippet || page.summary || page.section || "", terms);
      link.append(title, snippet);
      item.append(link);
      resultsList.append(item);
    });
    panel.hidden = false;
    input.setAttribute("aria-expanded", "true");
    input.removeAttribute("aria-activedescendant");
    activeIndex = -1;
  };

  const renderStatusResult = (message) => {
    resultsList.replaceChildren();
    const item = document.createElement("li");
    const status = document.createElement("span");
    item.className = "search-status";
    item.setAttribute("role", "option");
    item.setAttribute("aria-selected", "false");
    status.textContent = message;
    item.append(status);
    resultsList.append(item);
    panel.hidden = false;
    input.setAttribute("aria-expanded", "true");
    input.removeAttribute("aria-activedescendant");
    activeIndex = -1;
  };

  const runSearch = async () => {
    const query = input.value.trim().toLowerCase();
    if (query.length < 2) {
      closeResults();
      resultsList.replaceChildren();
      return;
    }
    try {
      const terms = query.split(/\s+/).filter(Boolean);
      const loadedPages = await loadIndex();
      const matches = loadedPages
        .map((page) => {
          const title = (page.title || "").toLowerCase();
          const haystack = `${title} ${page.section || ""} ${page.summary || ""} ${page.content || ""}`.toLowerCase();
          if (!terms.every((term) => haystack.includes(term))) {
            return null;
          }
          const score = terms.reduce((total, term) => total + (title.includes(term) ? 3 : 1), 0);
          return { page, score, snippet: searchSnippet(page, terms) };
        })
        .filter(Boolean)
        .sort((left, right) => right.score - left.score || left.page.title.localeCompare(right.page.title))
        .slice(0, 8);
      renderResults(matches, terms);
    } catch {
      renderStatusResult("Search unavailable");
    }
  };

  document.addEventListener("keydown", (event) => {
    if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) {
      return;
    }
    if (event.key === "/" && !isEditable(event.target)) {
      event.preventDefault();
      input.focus();
      input.select();
      runSearch();
      return;
    }
    if (event.key === "Escape" && (document.activeElement === input || input.value || !panel.hidden)) {
      event.preventDefault();
      clearOrBlurSearch();
    }
  });

  input.addEventListener("input", runSearch);
  input.addEventListener("focus", runSearch);
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const items = Array.from(resultsList.querySelectorAll("[role='option'] a"));
    if (activeIndex >= 0 && items[activeIndex]) {
      items[activeIndex].click();
      return;
    }
    if (items[0]) {
      items[0].click();
    }
  });
  input.addEventListener("keydown", (event) => {
    const items = Array.from(resultsList.querySelectorAll("[role='option'] a"));
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      clearOrBlurSearch();
      return;
    }
    if (event.key === "ArrowDown" && items.length > 0) {
      event.preventDefault();
      setActive(activeIndex + 1);
      return;
    }
    if (event.key === "ArrowUp" && items.length > 0) {
      event.preventDefault();
      setActive(activeIndex - 1);
      return;
    }
    if (event.key === "Enter" && activeIndex >= 0 && items[activeIndex]) {
      event.preventDefault();
      items[activeIndex].click();
    }
  });
  document.addEventListener("click", (event) => {
    if (!form.contains(event.target)) {
      closeResults();
    }
  });
}

function appendHighlightedText(parent, value, terms) {
  const text = String(value);
  const pattern = highlightPattern(terms);
  if (!pattern) {
    parent.textContent = text;
    return;
  }
  let offset = 0;
  for (const match of text.matchAll(pattern)) {
    if (match.index > offset) {
      parent.append(document.createTextNode(text.slice(offset, match.index)));
    }
    const mark = document.createElement("mark");
    mark.className = "search-match";
    mark.textContent = match[0];
    parent.append(mark);
    offset = match.index + match[0].length;
  }
  if (offset < text.length) {
    parent.append(document.createTextNode(text.slice(offset)));
  }
}

function highlightPattern(terms) {
  const uniqueTerms = Array.from(new Set(terms.filter(Boolean))).sort((left, right) => right.length - left.length);
  if (uniqueTerms.length === 0) {
    return null;
  }
  return new RegExp(uniqueTerms.map(escapeRegExp).join("|"), "gi");
}

function searchSnippet(page, terms) {
  const source = normalizeSearchText(page.content || page.summary || page.section || "");
  if (!source) {
    return page.section || "";
  }
  const lowerSource = source.toLowerCase();
  const firstIndex = terms.reduce((best, term) => {
    const index = lowerSource.indexOf(term);
    if (index < 0) {
      return best;
    }
    return best < 0 ? index : Math.min(best, index);
  }, -1);
  if (firstIndex < 0) {
    return truncateSnippet(source, 180);
  }
  const context = 84;
  const start = Math.max(0, firstIndex - context);
  const end = Math.min(source.length, firstIndex + context);
  const prefix = start > 0 ? "... " : "";
  const suffix = end < source.length ? " ..." : "";
  return `${prefix}${source.slice(start, end).trim()}${suffix}`;
}

function normalizeSearchText(value) {
  return String(value).replace(/\s+/g, " ").trim();
}

function truncateSnippet(value, length) {
  const text = normalizeSearchText(value);
  if (text.length <= length) {
    return text;
  }
  return `${text.slice(0, length).trim()} ...`;
}

function escapeRegExp(value) {
  return String(value).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function isEditable(target) {
  if (!(target instanceof HTMLElement)) {
    return false;
  }
  const tagName = target.tagName.toLowerCase();
  return target.isContentEditable || tagName === "input" || tagName === "textarea" || tagName === "select";
}

function storageGet(storageName, key) {
  try {
    return window[storageName].getItem(key);
  } catch {
    return null;
  }
}

function storageSet(storageName, key, value) {
  try {
    window[storageName].setItem(key, value);
  } catch {
    // Storage can be disabled by browser privacy settings. The site still works without it.
  }
}
