document.addEventListener('DOMContentLoaded', () => {
    fetchTopics(false); // Initial load: try cache first

    // -- Mini Strip Navigation --
    document.getElementById('btn-show-topics').addEventListener('click', () => {
        toggleSidebar('sidebar', 'btn-show-topics');
    });

    document.getElementById('btn-show-headers').addEventListener('click', () => {
        toggleSidebar('headers-sidebar', 'btn-show-headers');
    });

    document.getElementById('btn-show-health').addEventListener('click', () => {
        toggleSidebar('secondary-sidebar', 'btn-show-health');
    });

    document.getElementById('btn-show-sources').addEventListener('click', () => {
        toggleSidebar('right-sidebar', 'btn-show-sources');
    });

    // -- Collapse Buttons --
    document.getElementById('collapse-topics-btn').addEventListener('click', () => {
        toggleSidebar('sidebar', 'btn-show-topics');
    });

    document.getElementById('collapse-headers-btn').addEventListener('click', () => {
        toggleSidebar('headers-sidebar', 'btn-show-headers');
    });

    document.getElementById('collapse-health-btn').addEventListener('click', () => {
        toggleSidebar('secondary-sidebar', 'btn-show-health');
    });

    document.getElementById('refresh-btn').addEventListener('click', () => {
        fetchTopics(true); // Explicit refresh: fetch new
    });

    document.getElementById('sort-num-btn').addEventListener('click', () => {
        setSort('number');
    });

    document.getElementById('sort-cite-btn').addEventListener('click', () => {
        setSort('citations');
    });
});

function toggleSidebar(sidebarId, btnId) {
    const sidebar = document.getElementById(sidebarId);
    const btn = document.getElementById(btnId);

    // Toggle the target sidebar
    const isCollapsed = sidebar.classList.toggle('collapsed');

    // Update button active state
    if (btn) btn.classList.toggle('active', !isCollapsed);
}

function closeSidebar(sidebarId, btnId) {
    const sidebar = document.getElementById(sidebarId);
    const btn = document.getElementById(btnId);
    if (sidebar) sidebar.classList.add('collapsed');
    if (btn) btn.classList.remove('active');
}

let currentArticles = [];
let currentCitations = {};
let currentSort = 'number';

async function fetchTopics(isRefresh = true) {
    const topicList = document.getElementById('topic-list');
    const loadingState = document.getElementById('loading-topics');
    const errorState = document.getElementById('error-topics');
    const statusText = document.getElementById('status-text-topics');

    topicList.classList.add('hidden');
    errorState.classList.add('hidden');
    loadingState.classList.remove('hidden');

    if (!isRefresh) {
        statusText.innerText = "Loading cached topics...";
    } else {
        statusText.innerText = "Clustering latest headlines...";
    }

    try {
        const limitVal = document.getElementById('feed-limit').value || 25;
        const res = await fetch(`/api/topics?limit=${limitVal}&refresh=${isRefresh}`);

        if (res.status === 429) {
            const data = await res.json();
            if (data.feed_stats) {
                renderFeedHealth(data.feed_stats);
                closeSidebar('sidebar', 'btn-show-topics');
                toggleSidebar('secondary-sidebar', 'btn-show-health');
            }
            handleTopicsRateLimit(data.retry_after);
            return;
        }

        if (!res.ok) throw new Error('Failed to fetch topics');

        const data = await res.json();
        renderTopics(data.clusters);
        renderFeedHealth(data.feed_stats);

        loadingState.classList.add('hidden');
        topicList.classList.remove('hidden');
    } catch (err) {
        console.error(err);
        loadingState.classList.add('hidden');
        errorState.classList.remove('hidden');
        errorState.innerHTML = `
            <p>Failed to load topics.</p>
            <button class="retry-btn" onclick="fetchTopics(true)">Try Again</button>
        `;
    }
}

let lastSelectedTitle = null;

function renderTopics(topics) {
    const list = document.getElementById('topic-list');
    list.innerHTML = ''; // Clear existing

    if (!topics || topics.length === 0) {
        list.innerHTML = `<li class="empty-list">No major topics found today.</li>`;
        return;
    }

    // Map with original index before sorting
    const enrichedTopics = topics.map((t, i) => ({ ...t, originalIndex: i }));

    // Sort: Selected first, then Cached Digest, then the rest
    const sortedTopics = enrichedTopics.sort((a, b) => {
        if (a.title === lastSelectedTitle) return -1;
        if (b.title === lastSelectedTitle) return 1;

        if (a.has_cached_digest && !b.has_cached_digest) return -1;
        if (!a.has_cached_digest && b.has_cached_digest) return 1;

        return 0;
    });

    sortedTopics.forEach((topic) => {
        const li = document.createElement('li');
        li.className = 'topic-card';
        if (topic.title === lastSelectedTitle) li.classList.add('active');

        let sourcesHTML = '';
        if (topic.source_counts) {
            for (const [source, sCount] of Object.entries(topic.source_counts)) {
                sourcesHTML += `<span class="source-bubble">${source} <span class="bubble-count">${sCount}</span></span>`;
            }
        } else {
            sourcesHTML = '<span class="source-bubble">Unknown</span>';
        }

        const cacheBadge = topic.has_cached_digest ? `<span class="cache-badge">Cached</span>` : '';

        li.innerHTML = `
            <div class="topic-header-row">
                <h4>${topic.title}</h4>
                ${cacheBadge}
            </div>
            <p class="desc">${topic.description}</p>
            <div class="meta">
                <div class="source-list">${sourcesHTML}</div>
            </div>
        `;

        li.addEventListener('click', () => {
            lastSelectedTitle = topic.title;
            // Immediate re-render to move to top
            renderTopics(topics);
            generateDigest(topic.originalIndex, topic.title);
        });

        list.appendChild(li);
    });
}

async function generateDigest(topicIdx, title) {
    const emptyState = document.getElementById('empty-state');
    const loadingState = document.getElementById('loading-digest');
    const digestView = document.getElementById('digest-view');
    const contentBox = document.getElementById('digest-content');
    const titleBox = document.getElementById('digest-title');
    const rightSidebar = document.getElementById('right-sidebar');

    // UI transitions
    emptyState.classList.add('hidden');
    digestView.classList.add('hidden');

    // Reset sidebars/panels
    loadingState.classList.remove('hidden');
    contentBox.innerHTML = '';

    document.getElementById('loading-status-text').innerText = `Preparing sources for: ${title}...`;
    document.getElementById('progress-step').innerText = "Initializing...";

    try {
        const response = await fetch(`/api/digest?topicId=${topicIdx}`, { method: 'POST' });

        if (!response.ok) {
            const errBody = await response.json().catch(() => ({}));
            throw new Error(errBody.error || `HTTP ${response.status}`);
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
            const { value, done } = await reader.read();
            if (done) break;

            buffer += decoder.decode(value, { stream: true });

            // Process SSE chunks
            const events = buffer.split('\n\n');
            buffer = events.pop(); // Keep partial chunk in buffer

            for (const rawEvent of events) {
                if (!rawEvent.trim()) continue;

                const lines = rawEvent.split('\n');
                let eventType = 'message';
                let dataStr = '';

                for (const line of lines) {
                    if (line.startsWith('event: ')) {
                        eventType = line.replace('event: ', '').trim();
                    } else if (line.startsWith('data: ')) {
                        dataStr = line.replace('data: ', '').trim();
                    }
                }

                if (dataStr) {
                    const data = JSON.parse(dataStr);

                    if (eventType === 'progress') {
                        document.getElementById('progress-step').innerText = `Step ${data.step} of 3`;
                        document.getElementById('loading-status-text').innerText = data.message;
                    } else if (eventType === 'error') {
                        if (data.error === "API Rate Limited") {
                            handleDigestRateLimit(data.retry_after, () => generateDigest(topicIdx, title));
                        } else {
                            throw new Error(data.message || "Failed to generate digest");
                        }
                        return;
                    } else if (eventType === 'result') {
                        // Render final result
                        titleBox.innerText = data.title;
                        contentBox.innerHTML = marked.parse(data.digest);

                        if (data.articles) {
                            linkifyCitations(contentBox, data.articles);
                        }

                        // Generate Navigation Headers
                        renderHeaderNav(data.digest);

                        loadingState.classList.add('hidden');
                        digestView.classList.remove('hidden');

                        if (data.articles) {
                            currentArticles = data.articles;
                            currentCitations = countCitations(data.digest, data.articles);
                            window.lastSkippedArticles = data.skipped_articles || [];
                            renderSourceArticles(window.lastSkippedArticles);

                            if (data.feed_stats) {
                                renderFeedHealth(data.feed_stats);
                            }

                            document.getElementById('digest-sources-panel').classList.remove('hidden');

                            // Automatically show sources when digest is ready
                            const rs = document.getElementById('right-sidebar');
                            if (rs) {
                                rs.classList.remove('collapsed');
                                const btnS = document.getElementById('btn-show-sources');
                                if (btnS) btnS.classList.add('active');
                            }

                            bindCitationLinks(data.articles);
                        }
                    }
                }
            }
        }
    } catch (err) {
        console.error(err);
        loadingState.classList.add('hidden');
        digestView.classList.remove('hidden');
        // Keep right sidebar state as is or hide it
        document.getElementById('digest-sources-panel').classList.add('hidden');
        titleBox.innerText = 'Error Generating Digest';
        contentBox.innerHTML = `<div class="error-box"><p>${err.message}</p></div>`;
    }
}

function renderFeedHealth(stats) {
    const list = document.getElementById('feed-health-list');
    list.innerHTML = '';

    if (!stats) return;

    Object.keys(stats).sort().forEach(source => {
        const item = stats[source];
        const hasError = !!item.error;
        let colorClass = 'status-ok';
        let icon = '✅';

        // Build description: prioritize "N new / M cached" if available
        let descParts = [];
        if (item.new_count > 0 || item.cached_count > 0) {
            descParts.push(`${item.new_count || 0} new`);
            descParts.push(`${item.cached_count || 0} cached`);
        } else if (item.article_count > 0) {
            descParts.push(`${item.article_count} item(s)`);
        }

        let desc = descParts.length > 0 ? descParts.join(' / ') : '0 articles found';

        if (item.skipped_count > 0) {
            desc += ` / <span style="color:#f85149; font-weight:bold;">${item.skipped_count} skipped</span>`;
            if (colorClass === 'status-ok') colorClass = 'status-warn';
            if (icon === '✅') icon = '⚠️';
        }

        if (hasError) {
            colorClass = 'status-error';
            icon = '❌';
            desc = item.error;
        } else if (item.article_count === 0 && item.cached_count === 0) {
            colorClass = 'status-warn';
            icon = '⚠️';
            desc = '0 articles fetched';
        }

        const li = document.createElement('li');
        li.className = `source-article-card feed-health-card ${colorClass}`;
        li.innerHTML = `
            <div class="col-meta">
                <span class="health-icon">${icon}</span> <strong>${item.name}</strong>
            </div>
            <div class="health-desc" style="margin-top:4px; font-size:12px; color:var(--text-secondary); word-break: break-all;">${desc}</div>
        `;
        list.appendChild(li);
    });
}

function renderSourceArticles(skippedArticles = []) {
    const list = document.getElementById('source-list-panel');
    const countLabel = document.getElementById('source-count');
    list.innerHTML = '';

    if (!currentArticles || currentArticles.length === 0) {
        countLabel.innerText = '0 articles found';
        return;
    }

    // Determine how many were actually scraped vs skipped
    const normalizeUrl = (u) => u.split('?')[0].split('#')[0].trim();
    const skippedSet = new Set((skippedArticles || []).map(sa => normalizeUrl(sa.Link)));
    const scrapedCount = currentArticles.filter(a => !skippedSet.has(normalizeUrl(a.Link))).length;

    countLabel.innerText = `${currentArticles.length} found (${scrapedCount} scraped / ${skippedSet.size} skipped)`;

    // Map cluster articles to their original index, citation count, and skipped status
    let sorted = currentArticles.map((art, idx) => ({
        ...art,
        originalIndex: idx + 1,
        citations: currentCitations[idx + 1] || 0,
        skipped: skippedSet.has(normalizeUrl(art.Link))
    }));

    if (currentSort === 'citations') {
        sorted.sort((a, b) => b.citations - a.citations || a.originalIndex - b.originalIndex);
    } else {
        sorted.sort((a, b) => a.originalIndex - b.originalIndex);
    }

    sorted.forEach((art) => {
        const li = document.createElement('li');
        li.className = 'source-article-card' + (art.skipped ? ' skipped' : '');
        li.id = `source-card-${art.originalIndex}`;

        const citationBadge = (art.citations > 0 && !art.skipped)
            ? `<span class="citation-badge">${art.citations} cit.</span>`
            : '';

        const statusText = art.skipped ? ' [SKIPPED/PAYWALL]' : '';

        li.innerHTML = `
            <div class="col-meta">
                <span class="article-ref">[Article ${art.originalIndex}]</span> 
                ${art.SourceName}
                ${citationBadge}
                ${statusText}
            </div>
            <a href="${art.Link}" target="_blank" class="col-title">${art.Title}</a>
        `;
        list.appendChild(li);
    });
}

function setSort(type) {
    currentSort = type;
    document.getElementById('sort-num-btn').classList.toggle('active', type === 'number');
    document.getElementById('sort-cite-btn').classList.toggle('active', type === 'citations');
    renderSourceArticles(window.lastSkippedArticles || []);
}

function countCitations(text, articles) {
    const counts = {};
    const regex = /\[(\d+(?:,\s*\d+)*)\]/g;
    let match;
    while ((match = regex.exec(text)) !== null) {
        const nums = match[1].split(',').map(n => parseInt(n.trim()));
        nums.forEach(n => {
            if (!isNaN(n)) {
                counts[n] = (counts[n] || 0) + 1;
            }
        });
    }
    return counts;
}

function scrollToSource(n) {
    const allCards = document.querySelectorAll('.source-article-card');
    allCards.forEach(c => c.classList.remove('highlight'));

    const card = document.getElementById(`source-card-${n}`);
    if (card) {
        const rs = document.getElementById('right-sidebar');
        if (rs && rs.classList.contains('collapsed')) {
            rs.classList.remove('collapsed');
            const btnS = document.getElementById('btn-show-sources');
            if (btnS) btnS.classList.add('active');
        }
        card.classList.add('highlight');
        card.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }
}

function linkifyCitations(container, articles) {
    const citationRegex = /\[(\d+(?:,\s*\d+)*)\]/g;

    const walker = document.createTreeWalker(container, NodeFilter.SHOW_TEXT, null);
    const textNodes = [];
    let node;
    while ((node = walker.nextNode())) {
        if (citationRegex.test(node.textContent)) textNodes.push(node);
        citationRegex.lastIndex = 0;
    }

    textNodes.forEach(textNode => {
        const parent = textNode.parentNode;
        const text = textNode.textContent;
        const fragment = document.createDocumentFragment();
        let lastIndex = 0;
        citationRegex.lastIndex = 0;
        let match;

        while ((match = citationRegex.exec(text)) !== null) {
            if (match.index > lastIndex) {
                fragment.appendChild(document.createTextNode(text.slice(lastIndex, match.index)));
            }

            const nums = match[1].split(',').map(n => parseInt(n.trim())).filter(n => n >= 1 && n <= articles.length);

            if (nums.length === 0) {
                fragment.appendChild(document.createTextNode(match[0]));
            } else {
                fragment.appendChild(document.createTextNode('['));
                nums.forEach((n, i) => {
                    if (i > 0) fragment.appendChild(document.createTextNode(', '));
                    const span = document.createElement('span');
                    span.className = 'citation-link';
                    span.dataset.article = n;
                    span.textContent = n;
                    span.addEventListener('click', () => scrollToSource(n));
                    fragment.appendChild(span);
                });
                fragment.appendChild(document.createTextNode(']'));
            }

            lastIndex = match.index + match[0].length;
        }

        if (lastIndex < text.length) {
            fragment.appendChild(document.createTextNode(text.slice(lastIndex)));
        }

        parent.replaceChild(fragment, textNode);
    });
}

function bindCitationLinks(articles) {
    const contentBox = document.getElementById('digest-content');

    contentBox.querySelectorAll('a').forEach(anchor => {
        anchor.addEventListener('click', (e) => {
            const href = anchor.getAttribute('href');
            const sourceIdx = articles.findIndex(a => a.Link === href || href.includes(a.Link));

            if (sourceIdx !== -1) {
                e.preventDefault();
                scrollToSource(sourceIdx + 1);
            }
        });
    });
}

function handleTopicsRateLimit(seconds) {
    const loadingState = document.getElementById('loading-topics');
    const errorState = document.getElementById('error-topics');

    loadingState.classList.add('hidden');
    errorState.classList.remove('hidden');

    let remaining = seconds || 60;

    const updateUI = () => {
        errorState.innerHTML = `
            <p style="color:var(--status-warn);">API Rate Limited.</p>
            <div style="font-size: 13px; font-weight: bold; margin-top: 10px;">
                Retrying in ${remaining}s...
            </div>
        `;
    };

    updateUI();
    const intv = setInterval(() => {
        remaining--;
        if (remaining <= 0) {
            clearInterval(intv);
            errorState.innerHTML = `<p>Retrying...</p>`;
            fetchTopics();
        } else {
            updateUI();
        }
    }, 1000);
}

function handleDigestRateLimit(seconds, retryCallback) {
    const loadingState = document.getElementById('loading-digest');
    const digestView = document.getElementById('digest-view');
    const titleBox = document.getElementById('digest-title');
    const contentBox = document.getElementById('digest-content');

    loadingState.classList.add('hidden');
    digestView.classList.remove('hidden');

    titleBox.innerText = 'Rate Limit Exceeded (429)';

    let remaining = seconds || 60;

    const updateUI = () => {
        contentBox.innerHTML = `
            <div class="error-box" style="border-left-color: var(--status-warn); background: rgba(234, 179, 8, 0.1);">
                <p>Gemini API free quota exceeded. Waiting to retry...</p>
                <div style="font-size: 24px; font-weight: bold; margin-top: 15px; text-align: center; color: var(--status-warn);">
                    Retrying in ${remaining}s
                </div>
            </div>`;
    };

    updateUI();

    const intv = setInterval(() => {
        remaining--;
        if (remaining <= 0) {
            clearInterval(intv);
            contentBox.innerHTML = `
                <div class="error-box">
                    <p>Retrying now...</p>
                </div>`;
            retryCallback();
        } else {
            updateUI();
        }
    }, 1000);
}

function renderHeaderNav(markdown) {
    const navList = document.getElementById('header-nav-list');
    navList.innerHTML = '';

    const contentBox = document.getElementById('digest-content');

    // First pass: inject IDs into headers in the rendered DOM
    const allHeaders = contentBox.querySelectorAll('h2, h3, h4');
    allHeaders.forEach((h, idx) => {
        h.id = `digest-h-${idx}`;
    });

    let headerCount = 0;
    allHeaders.forEach((h, idx) => {
        headerCount++;
        const level = h.tagName.toLowerCase(); // 'h1', 'h2', 'h3'
        const title = h.innerText.replace(/\[Article\s+[\d,\s]+\]/g, '').trim();

        const li = document.createElement('li');
        li.className = `header-nav-item ${level}`;
        li.innerText = title;
        li.addEventListener('click', () => {
            const target = document.getElementById(`digest-h-${idx}`);
            const container = document.querySelector('.main-content');
            if (target && container) {
                const targetTop = target.offsetTop;
                container.scrollTo({
                    top: targetTop - 40, // Offset for some breathing room
                    behavior: 'smooth'
                });
                document.querySelectorAll('.header-nav-item').forEach(n => n.classList.remove('active'));
                li.classList.add('active');
            }
        });
        navList.appendChild(li);
    });

    if (headerCount === 0) {
        navList.innerHTML = `<div class="empty-nav-msg">No headers found in this digest.</div>`;
    }
}
