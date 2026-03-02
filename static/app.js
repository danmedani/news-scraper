document.addEventListener('DOMContentLoaded', () => {
    fetchTopics();

    document.getElementById('refresh-btn').addEventListener('click', () => {
        fetchTopics();
    });

    document.getElementById('toggle-health-btn').addEventListener('click', () => {
        const sidebar = document.getElementById('secondary-sidebar');
        const caret = document.getElementById('health-caret');
        sidebar.classList.toggle('collapsed');
        if (sidebar.classList.contains('collapsed')) {
            caret.innerText = '▼';
        } else {
            caret.innerText = '▲';
        }
    });

    document.getElementById('sort-num-btn').addEventListener('click', () => {
        setSort('number');
    });

    document.getElementById('sort-cite-btn').addEventListener('click', () => {
        setSort('citations');
    });
});

let currentArticles = [];
let currentCitations = {};
let currentSort = 'number';

async function fetchTopics() {
    const topicList = document.getElementById('topic-list');
    const loadingState = document.getElementById('loading-topics');
    const errorState = document.getElementById('error-topics');

    topicList.classList.add('hidden');
    errorState.classList.add('hidden');
    loadingState.classList.remove('hidden');

    try {
        const limitVal = document.getElementById('feed-limit').value || 25;
        const res = await fetch(`/api/topics?limit=${limitVal}`);

        if (res.status === 429) {
            const data = await res.json();
            if (data.feed_stats) {
                renderFeedHealth(data.feed_stats);
                document.getElementById('secondary-sidebar').classList.remove('collapsed');
                document.getElementById('health-caret').innerText = '▲';
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
            <button class="retry-btn" onclick="fetchTopics()">Try Again</button>
        `;
    }
}

function renderTopics(topics) {
    const list = document.getElementById('topic-list');
    list.innerHTML = ''; // Clear existing

    if (!topics || topics.length === 0) {
        list.innerHTML = `<li class="empty-list">No major topics found today.</li>`;
        return;
    }

    topics.forEach((topic, idx) => {
        const li = document.createElement('li');
        li.className = 'topic-card';

        let sourcesHTML = '';
        if (topic.source_counts) {
            for (const [source, sCount] of Object.entries(topic.source_counts)) {
                sourcesHTML += `<span class="source-bubble">${source} <span class="bubble-count">${sCount}</span></span>`;
            }
        } else {
            sourcesHTML = '<span class="source-bubble">Unknown</span>';
        }

        const count = topic.article_indices ? topic.article_indices.length : 0;

        li.innerHTML = `
            <h4>${topic.title}</h4>
            <p class="desc">${topic.description}</p>
            <div class="meta">
                <div class="source-list">${sourcesHTML}</div>
            </div>
        `;

        li.addEventListener('click', () => {
            // Remove active state from all, add to this one
            document.querySelectorAll('.topic-card').forEach(n => n.classList.remove('active'));
            li.classList.add('active');

            generateDigest(idx, topic.title);
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
    if (rightSidebar) rightSidebar.classList.add('hidden');
    document.getElementById('digest-sources-panel').classList.add('hidden');
    loadingState.classList.remove('hidden');
    contentBox.innerHTML = '';

    document.getElementById('loading-digest-topic').innerText = `Synthesizing report for: ${title}...`;

    try {
        const res = await fetch(`/api/digest?topicId=${topicIdx}`, { method: 'POST' });

        if (res.status === 429) {
            const data = await res.json();
            handleDigestRateLimit(data.retry_after, () => generateDigest(topicIdx, title));
            return;
        }

        if (!res.ok) {
            const errBody = await res.json().catch(() => ({}));
            throw new Error(errBody.error || `HTTP ${res.status}`);
        }

        const data = await res.json();

        // Render Markdown
        titleBox.innerText = data.title;
        contentBox.innerHTML = marked.parse(data.digest);

        loadingState.classList.add('hidden');
        digestView.classList.remove('hidden');

        if (data.articles) {
            currentArticles = data.articles;
            currentCitations = countCitations(data.digest, data.articles);

            // Render source articles including skipped ones
            window.lastSkippedArticles = data.skipped_articles || [];
            renderSourceArticles(window.lastSkippedArticles);

            // Update feed health with latest skipped counts
            if (data.feed_stats) {
                renderFeedHealth(data.feed_stats);
            }

            document.getElementById('digest-sources-panel').classList.remove('hidden');
            if (rightSidebar) rightSidebar.classList.remove('hidden');
            bindCitationLinks(data.articles);
        }
    } catch (err) {
        console.error(err);
        loadingState.classList.add('hidden');
        digestView.classList.remove('hidden');
        if (rightSidebar) rightSidebar.classList.add('hidden');
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
    // Look for [Article X, Y] or [Article X] patterns
    const regex = /\[Article\s+([\d,\s]+)\]/g;
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

function bindCitationLinks(articles) {
    const contentBox = document.getElementById('digest-content');

    contentBox.querySelectorAll('a').forEach(anchor => {
        anchor.addEventListener('click', (e) => {
            const href = anchor.getAttribute('href');

            const sourceIdx = articles.findIndex(a => a.Link === href || href.includes(a.Link));

            if (sourceIdx !== -1) {
                e.preventDefault();

                const allCards = document.querySelectorAll('.source-article-card');
                allCards.forEach(c => c.classList.remove('highlight'));

                const card = document.getElementById(`source-card-${sourceIdx + 1}`);
                if (card) {
                    card.classList.add('highlight');
                    card.scrollIntoView({ behavior: 'smooth', block: 'center' });
                }
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
