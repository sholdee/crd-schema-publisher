document.addEventListener("DOMContentLoaded", () => {
  const searchForm = document.querySelector(".site-search");
  if (searchForm) {
    initSearch(searchForm);
  }

  const sidebar = document.querySelector(".site-sidebar");
  if (sidebar) {
    initSidebarPersistence(sidebar);
  }

  initGitHubCard();
  initShellCommandHighlighting();
  initAccessibleTables();

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

function initAccessibleTables() {
  document.querySelectorAll(".doc-content table").forEach((table, index) => {
    if (!table.hasAttribute("tabindex")) {
      table.tabIndex = 0;
    }
    if (!table.hasAttribute("aria-label")) {
      const label = nearestTableHeading(table) || `Data table ${index + 1}`;
      table.setAttribute("aria-label", label);
    }
  });
}

function nearestTableHeading(table) {
  let current = table.previousElementSibling;
  while (current) {
    if (/^H[2-6]$/.test(current.tagName)) {
      return `${current.textContent.trim()} table`;
    }
    current = current.previousElementSibling;
  }
  return "";
}

const GITHUB_CARD_CACHE_TTL_MS = 6 * 60 * 60 * 1000;

function initGitHubCard() {
  const card = document.querySelector(".github-card");
  if (!card || !card.dataset.githubRepo || !card.dataset.githubRepositoryApi || !window.fetch) {
    return;
  }

  const cacheKey = `crdsp.github-card.${card.dataset.githubRepo}`;
  const cached = githubCardCachedData(cacheKey);
  if (cached && Date.now() - cached.cachedAt < GITHUB_CARD_CACHE_TTL_MS) {
    renderGitHubCard(card, cached);
    return;
  }

  refreshGitHubCard(card, cacheKey);
}

async function refreshGitHubCard(card, cacheKey) {
  try {
    const repo = await fetchGitHubJSON(card.dataset.githubRepositoryApi);
    let release = null;
    if (card.dataset.githubReleaseApi) {
      try {
        release = await fetchGitHubJSON(card.dataset.githubReleaseApi);
      } catch {
        release = null;
      }
    }

    const data = {
      repo: repo.full_name || card.dataset.githubRepo,
      stars: normalizeGitHubCount(repo.stargazers_count),
      forks: normalizeGitHubCount(repo.forks_count),
      version: normalizeGitHubVersion(release?.tag_name || githubFieldText(card, "version")),
      cachedAt: Date.now(),
    };
    renderGitHubCard(card, data);
    storageSet("localStorage", cacheKey, JSON.stringify(data));
  } catch {
    // The fallback card remains useful when GitHub is unavailable or rate-limited.
  }
}

async function fetchGitHubJSON(url) {
  const response = await fetch(url, {
    headers: { Accept: "application/vnd.github+json" },
  });
  if (!response.ok) {
    throw new Error(`GitHub API unavailable: ${response.status}`);
  }
  return response.json();
}

function githubCardCachedData(cacheKey) {
  try {
    const parsed = JSON.parse(storageGet("localStorage", cacheKey) || "null");
    if (!parsed || typeof parsed !== "object" || !Number.isFinite(parsed.cachedAt)) {
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

function renderGitHubCard(card, data) {
  const repo = data.repo || card.dataset.githubRepo || githubFieldText(card, "repo");
  const version = normalizeGitHubVersion(data.version || githubFieldText(card, "version"));
  const stars = formatGitHubCount(data.stars ?? githubFieldText(card, "stars"));
  const forks = formatGitHubCount(data.forks ?? githubFieldText(card, "forks"));

  setGitHubFieldText(card, "repo", repo);
  setGitHubFieldText(card, "version", version);
  setGitHubFieldText(card, "stars", stars);
  setGitHubFieldText(card, "forks", forks);
  card.setAttribute("aria-label", `${repo} on GitHub, latest release ${version}, ${stars} stars, ${forks} forks`);
}

function setGitHubFieldText(card, field, value) {
  const target = card.querySelector(`[data-github-field='${field}']`);
  if (target) {
    target.textContent = value;
  }
}

function githubFieldText(card, field) {
  return card.querySelector(`[data-github-field='${field}']`)?.textContent.trim() || "";
}

function normalizeGitHubVersion(value) {
  return String(value || "").trim().replace(/^v(?=\d)/i, "");
}

function normalizeGitHubCount(value) {
  const number = Number(value);
  return Number.isFinite(number) && number >= 0 ? number : 0;
}

function formatGitHubCount(value) {
  const number = Number(value);
  if (Number.isFinite(number) && number >= 0) {
    return number.toLocaleString("en-US");
  }
  return String(value || "0");
}

function initShellCommandHighlighting() {
  document.querySelectorAll(".doc-content code.language-bash, .doc-content code.language-sh").forEach((block) => {
    const lines = block.querySelectorAll(".cl");
    const targets = lines.length > 0 ? lines : [block];
    targets.forEach((line) => {
      wrapTextRanges(line, shellCommandRanges(line.textContent || ""), "shell-command");
    });
  });
}

function shellCommandRanges(text) {
  const ranges = [];
  let index = 0;
  let expectingCommand = true;

  while (index < text.length) {
    index = skipShellWhitespace(text, index);
    if (index >= text.length || text[index] === "#") {
      break;
    }

    if (expectingCommand) {
      if (text[index] === "$" && /\s/.test(text[index + 1] || "")) {
        index += 1;
        continue;
      }

      const assignmentEnd = shellAssignmentEnd(text, index);
      if (assignmentEnd > index) {
        index = assignmentEnd;
        continue;
      }

      const end = shellTokenEnd(text, index);
      const token = text.slice(index, end);
      if (isShellCommandToken(token)) {
        ranges.push({ start: index, end });
      }
      expectingCommand = false;
      index = end;
      continue;
    }

    const separatorEnd = shellSeparatorEnd(text, index);
    if (separatorEnd > index) {
      expectingCommand = true;
      index = separatorEnd;
      continue;
    }

    index += 1;
  }

  return ranges;
}

function skipShellWhitespace(text, index) {
  let next = index;
  while (next < text.length && /\s/.test(text[next])) {
    next += 1;
  }
  return next;
}

function shellAssignmentEnd(text, index) {
  const match = text.slice(index).match(/^[A-Za-z_][A-Za-z0-9_]*=/);
  if (!match) {
    return index;
  }
  return shellTokenEnd(text, index + match[0].length);
}

function shellTokenEnd(text, index) {
  let next = index;
  while (next < text.length && !/\s/.test(text[next]) && !/[|;&]/.test(text[next])) {
    next += 1;
  }
  return next;
}

function shellSeparatorEnd(text, index) {
  const nextTwo = text.slice(index, index + 2);
  if (nextTwo === "&&" || nextTwo === "||") {
    return index + 2;
  }
  return /[|;]/.test(text[index] || "") ? index + 1 : index;
}

function isShellCommandToken(token) {
  return /^[A-Za-z_./][A-Za-z0-9_./:-]*$/.test(token) && !token.startsWith("-");
}

function wrapTextRanges(container, ranges, className) {
  if (ranges.length === 0) {
    return;
  }

  const walker = document.createTreeWalker(container, NodeFilter.SHOW_TEXT);
  const textNodes = [];
  let offset = 0;
  let node = walker.nextNode();
  while (node) {
    const start = offset;
    const end = start + node.nodeValue.length;
    textNodes.push({ node, start, end });
    offset = end;
    node = walker.nextNode();
  }

  textNodes.forEach(({ node: textNode, start, end }) => {
    const overlaps = ranges
      .map((range) => ({
        start: Math.max(range.start, start) - start,
        end: Math.min(range.end, end) - start,
      }))
      .filter((range) => range.start < range.end);

    if (overlaps.length === 0) {
      return;
    }

    const fragment = document.createDocumentFragment();
    const value = textNode.nodeValue;
    let cursor = 0;
    overlaps.forEach((range) => {
      if (range.start > cursor) {
        fragment.append(document.createTextNode(value.slice(cursor, range.start)));
      }
      const span = document.createElement("span");
      span.className = className;
      span.textContent = value.slice(range.start, range.end);
      fragment.append(span);
      cursor = range.end;
    });
    if (cursor < value.length) {
      fragment.append(document.createTextNode(value.slice(cursor)));
    }
    textNode.replaceWith(fragment);
  });
}

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
