const { Pool } = require('pg');
const pool = new Pool({ connectionString: process.env.DATABASE_URL });

// Convert bigint count values to numbers (node-pg returns COUNT(*) as string)
const toNum = (v) => {
  const n = Number(v);
  return Number.isFinite(n) ? n : v;
};

module.exports = async (req, res) => {
  try {
    const r = {};
    const q = async (sql) => {
      const { rows } = await pool.query(sql);
      // Convert known count fields to numbers; handle nulls for run_log
      return rows.map(row => {
        if ('count' in row) row.count = toNum(row.count);
        if ('cnt' in row) row.cnt = toNum(row.cnt);
        if ('coalesce' in row) row.coalesce = toNum(row.coalesce);
        ['jobs_found','emails_queued','emails_sent','emails_failed'].forEach(k => {
          if (k in row) row[k] = (row[k] == null ? 0 : toNum(row[k]));
        });
        return row;
      });
    };

    // ── Aggregate pipeline stats (cumulative across all scrape runs) ──
    const agg = (await q("SELECT COALESCE(SUM(jobs_found),0) as total_raw, COALESCE(SUM(emails_queued),0) as total_inserted FROM run_log WHERE workflow='scrape'"))[0];
    r.total_raw = agg.total_raw;
    r.total_inserted = agg.total_inserted;
    r.filtered_deduped = r.total_raw - r.total_inserted;

    // ── Unique emails (distinct email+company pairs) ──
    r.unique_emails = (await q("SELECT COUNT(*) as count FROM (SELECT DISTINCT email_addr, company_name FROM email_queue) sub"))[0].count;

    // ── Pipeline funnel ──
    r.total_jobs = (await q("SELECT COUNT(*) FROM jobs"))[0].count;
    r.with_emails = (await q("SELECT COUNT(*) FROM jobs WHERE emails != '[]'::jsonb AND emails IS NOT NULL"))[0].count;
    r.scored = (await q("SELECT COUNT(*) FROM jobs WHERE llm_score IS NOT NULL"))[0].count;
    r.avg_score = (await q("SELECT COALESCE(ROUND(AVG(llm_score::numeric),1),0)::float FROM jobs WHERE llm_score IS NOT NULL"))[0].coalesce;
    r.researched = (await q("SELECT COUNT(*) FROM jobs WHERE research_notes IS NOT NULL"))[0].count;

    // ── Email queue breakdown ──
    r.queue_pending = (await q("SELECT COUNT(*) FROM email_queue WHERE status = 'pending'"))[0].count;
    r.queue_generating = (await q("SELECT COUNT(*) FROM email_queue WHERE status = 'generating'"))[0].count;
    r.queue_generated = (await q("SELECT COUNT(*) FROM email_queue WHERE status = 'generated'"))[0].count;
    r.queue_sent = (await q("SELECT COUNT(*) FROM email_queue WHERE status = 'sent'"))[0].count;
    r.queue_failed = (await q("SELECT COUNT(*) FROM email_queue WHERE status = 'failed'"))[0].count;

    // ── Tracking ──
    r.total_sent = (await q("SELECT COUNT(*) FROM tracking"))[0].count;
    r.opens = (await q("SELECT COUNT(*) FROM tracking WHERE opened = true"))[0].count;
    r.unique_opens = (await q("SELECT COUNT(*) FROM (SELECT DISTINCT email_id FROM tracking WHERE opened = true) sub"))[0].count;
    r.clicks = (await q("SELECT COUNT(*) FROM click_log"))[0].count;
    r.unique_clicked = (await q("SELECT COUNT(*) FROM (SELECT DISTINCT email_id FROM click_log) sub"))[0].count;
    r.open_pct = r.total_sent > 0 ? Math.round(r.unique_opens * 100 / r.total_sent) : 0;
    r.click_pct = r.total_sent > 0 ? Math.round(r.unique_clicked * 100 / r.total_sent) : 0;

    // ── Per-site breakdown ──
    const sites = await q("SELECT source_site, COUNT(*) as cnt FROM jobs GROUP BY source_site ORDER BY cnt DESC LIMIT 15");

    // ── Score distribution ──
    const scores = await q("SELECT llm_score, COUNT(*) as cnt FROM jobs WHERE llm_score IS NOT NULL GROUP BY llm_score ORDER BY llm_score");

    // ── Run history ──
    const runs = await q("SELECT workflow, mode, status, jobs_found, emails_queued, emails_sent, emails_failed, error_msg, to_char(started_at, 'Mon DD HH24:MI') as started FROM run_log ORDER BY started_at DESC LIMIT 15");

    // ── Recent failures ──
    const failures = await q("SELECT eq.email_addr, eq.company_name, eq.error_msg, to_char(eq.created_at, 'Mon DD HH24:MI') as when FROM email_queue eq WHERE eq.status = 'failed' AND eq.error_msg != '' ORDER BY eq.created_at DESC NULLS LAST LIMIT 10");

    // ── Click breakdown by URL ──
    const clickByUrl = await q("SELECT url, COUNT(*) as cnt FROM click_log GROUP BY url ORDER BY cnt DESC");

    // ── Recent clicks ──
    const clicks = await q("SELECT c.url, to_char(c.clicked_at, 'Mon DD HH24:MI') as when FROM click_log c ORDER BY c.clicked_at DESC LIMIT 10");

    const total_emails = r.queue_pending + r.queue_generating + r.queue_generated + r.queue_sent + r.queue_failed;

    // ── HTML ──
    const html = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>JobHunter Pipeline</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:'Inter',system-ui,sans-serif;background:oklch(0.13 0.01 260);color:oklch(0.92 0.005 260);padding:clamp(1rem,3vw,2.5rem);max-width:1280px;margin:0 auto;min-height:100dvh}
::selection{background:oklch(0.55 0.18 45);color:#000}

/* ── header ── */
header{margin-bottom:2rem;display:flex;align-items:baseline;gap:.75rem;flex-wrap:wrap}
h1{font-size:clamp(1.1rem,2.2vw,1.5rem);font-weight:700;letter-spacing:-.02em;color:oklch(0.95 0.01 260)}
h1 span{color:oklch(0.65 0.16 45)}
.sub{font-size:.82rem;color:oklch(0.6 0.02 260);font-weight:450}

/* ── pipeline bar ── */
.pipeline{display:flex;gap:.3rem;align-items:stretch;margin-bottom:2.5rem;flex-wrap:wrap}
.pipeline .seg{flex:1;min-width:60px;padding:.7rem .5rem .5rem;border-radius:6px;text-align:center;position:relative}
.pipeline .seg .num{font-size:clamp(1rem,1.8vw,1.3rem);font-weight:700;letter-spacing:-.02em;line-height:1}
.pipeline .seg .lbl{font-size:.65rem;font-weight:500;opacity:.7;margin-top:.25rem;letter-spacing:.03em;text-transform:uppercase}
.pipeline .seg .chg{position:absolute;top:-.25rem;right:.3rem;font-size:.6rem;opacity:.4}
.pipeline .arrow{display:flex;align-items:center;color:oklch(0.35 0.02 260);font-size:.8rem;padding:0 .1rem;user-select:none}
.pipeline .seg-0{background:oklch(0.18 0.02 260/.5)}
.pipeline .seg-1{background:oklch(0.55 0.2 260/.15);border:1px solid oklch(0.55 0.2 260/.3)}
.pipeline .seg-2{background:oklch(0.5 0.18 160/.15);border:1px solid oklch(0.5 0.18 160/.3)}
.pipeline .seg-3{background:oklch(0.55 0.18 45/.15);border:1px solid oklch(0.55 0.18 45/.3)}
.pipeline .seg-4{background:oklch(0.5 0.16 310/.15);border:1px solid oklch(0.5 0.16 310/.3)}
.pipeline .seg-5{background:oklch(0.5 0.18 160/.15);border:1px solid oklch(0.5 0.18 160/.3)}
.pipeline .seg-6{background:oklch(0.5 0.16 45/.15);border:1px solid oklch(0.5 0.16 45/.3)}
.pipeline .seg-7{background:oklch(0.5 0.16 10/.15);border:1px solid oklch(0.5 0.16 10/.3)}

/* ── grid sections ── */
.section{margin-bottom:2.5rem}
.section-header{display:flex;align-items:baseline;gap:.6rem;margin-bottom:.9rem}
.section-header h2{font-size:.9rem;font-weight:600;text-transform:uppercase;letter-spacing:.06em;color:oklch(0.7 0.02 260)}
.section-header .sub{font-size:.78rem;color:oklch(0.5 0.02 260);font-weight:400}

.stat-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(120px,1fr));gap:.6rem;margin-bottom:2rem}

.stat-card{background:oklch(0.2 0.015 260/.6);border:1px solid oklch(0.28 0.02 260/.5);border-radius:8px;padding:.85rem 1rem}
.stat-card .num{font-size:1.15rem;font-weight:700;letter-spacing:-.02em;color:oklch(0.9 0.01 260);line-height:1.2}
.stat-card .num.highlight{color:oklch(0.65 0.2 260)}
.stat-card .num.amber{color:oklch(0.7 0.16 70)}
.stat-card .num.green{color:oklch(0.65 0.18 150)}
.stat-card .num.red{color:oklch(0.65 0.18 25)}
.stat-card .num.purple{color:oklch(0.65 0.16 310)}
.stat-card .label{font-size:.7rem;color:oklch(0.55 0.02 260);margin-top:.15rem;font-weight:450}

/* ── tables ── */
.table-wrap{overflow-x:auto;border:1px solid oklch(0.25 0.015 260/.6);border-radius:8px}
table{width:100%;border-collapse:collapse;font-size:.8rem}
th{background:oklch(0.17 0.015 260);color:oklch(0.65 0.02 260);font-weight:600;text-transform:uppercase;letter-spacing:.04em;font-size:.7rem;padding:.6rem .7rem;text-align:left;border-bottom:1px solid oklch(0.25 0.015 260/.6);position:sticky;top:0}
td{padding:.5rem .7rem;border-bottom:1px solid oklch(0.22 0.01 260/.4);color:oklch(0.85 0.01 260)}
tr:last-child td{border-bottom:none}
tr:hover td{background:oklch(0.25 0.015 260/.4)}
.num-col{text-align:right;font-family:'JetBrains Mono',monospace;font-size:.75rem;font-variant-numeric:tabular-nums}
.col-status{min-width:80px}
.wrap{max-width:280px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}

.status-badge{display:inline-block;padding:.1rem .45rem;border-radius:3px;font-size:.68rem;font-weight:600;text-transform:uppercase;letter-spacing:.02em}
.status-badge.completed{background:oklch(0.5 0.18 160/.2);color:oklch(0.7 0.18 150)}
.status-badge.failed{background:oklch(0.5 0.18 25/.2);color:oklch(0.7 0.18 25)}
.status-badge.running{background:oklch(0.55 0.2 260/.2);color:oklch(0.7 0.2 260)}

/* ── score bars ── */
.score-row{display:flex;align-items:center;gap:.5rem;margin-bottom:.3rem}
.score-label{min-width:3rem;font-size:.75rem;color:oklch(0.6 0.02 260);font-weight:500}
.score-bar-wrap{flex:1;height:6px;background:oklch(0.25 0.015 260);border-radius:3px;overflow:hidden}
.score-bar{height:100%;border-radius:3px;transition:width .6s ease}
.score-bar.s0{background:oklch(0.6 0.15 10)}
.score-bar.s1{background:oklch(0.65 0.16 30)}
.score-bar.s2{background:oklch(0.7 0.16 70)}
.score-bar.s3{background:oklch(0.65 0.18 150)}
.score-bar.s4{background:oklch(0.55 0.2 260)}
.score-count{font-size:.75rem;font-family:'JetBrains Mono',monospace;color:oklch(0.55 0.02 260);min-width:2rem;text-align:right}

/* ── footer ── */
.footer{margin-top:3rem;padding-top:1rem;border-top:1px solid oklch(0.22 0.01 260/.4);font-size:.73rem;color:oklch(0.45 0.02 260);text-align:center;display:flex;gap:.75rem;justify-content:center;flex-wrap:wrap}
.footer a{color:oklch(0.55 0.16 260);text-decoration:none}
.footer a:hover{color:oklch(0.65 0.2 260);text-decoration:underline}

@media(max-width:640px){
  .stat-grid{grid-template-columns:repeat(2,1fr)}
  .pipeline .seg{min-width:50px;padding:.5rem .3rem .4rem}
  .pipeline .arrow{display:none}
}
</style></head>
<body>

<header>
  <h1>JobHunter <span>Pipeline</span></h1>
  <div class="sub">NeonDB · live</div>
</header>

<!-- ── Pipeline ── -->
<div class="section">
<div class="section-header"><h2>Pipeline</h2> <div class="sub">${r.total_raw.toLocaleString()} raw listings → ${r.unique_emails.toLocaleString()} unique emails</div></div>
<div class="pipeline">
  <div class="seg seg-0"><div class="num">${r.total_raw.toLocaleString()}</div><div class="lbl">Raw from scrappy</div></div>
  <div class="arrow">→</div>
  <div class="seg seg-1"><div class="num" style="color:oklch(0.65 0.18 25)">-${r.filtered_deduped.toLocaleString()}</div><div class="lbl">Filtered + Deduped</div></div>
  <div class="arrow">→</div>
  <div class="seg seg-2"><div class="num">${r.total_jobs.toLocaleString()}</div><div class="lbl">Unique jobs in DB</div></div>
  <div class="arrow">→</div>
  <div class="seg seg-3"><div class="num green">${r.unique_emails.toLocaleString()}</div><div class="lbl">Unique email+company</div></div>
  <div class="arrow">→</div>
  <div class="seg seg-4"><div class="num purple">${total_emails.toLocaleString()}</div><div class="lbl">Email Queue entries</div></div>
  <div class="arrow">→</div>
  <div class="seg seg-5"><div class="num green">${r.queue_sent.toLocaleString()}</div><div class="lbl">Sent</div></div>
  <div class="arrow">→</div>
  <div class="seg seg-6"><div class="num amber">${r.unique_opens.toLocaleString()}</div><div class="lbl">Opened</div></div>
  <div class="arrow">→</div>
  <div class="seg seg-7"><div class="num red">${r.unique_clicked.toLocaleString()}</div><div class="lbl">Clicked</div></div>
</div>
<p style="color:oklch(0.5 0.02 260);font-size:.72rem;margin-top:-1.5rem;margin-bottom:1.5rem">
  ${r.total_raw.toLocaleString()} raw scrappy listings → ${r.filtered_deduped.toLocaleString()} filtered/deduped → ${r.total_jobs.toLocaleString()} unique jobs stored
  → ${r.unique_emails.toLocaleString()} unique (email+company) pairs to send to
</p>
</div>

<!-- ── Email Queue + Engagement ── -->
<div style="display:grid;grid-template-columns:1fr 1fr;gap:1.5rem;margin-bottom:2.5rem">
<div class="section">
<div class="section-header"><h2>📬 Email Queue</h2></div>
<div class="stat-grid" style="grid-template-columns:repeat(auto-fill,minmax(90px,1fr))">
  <div class="stat-card"><div class="num highlight">${r.queue_pending.toLocaleString()}</div><div class="label">Pending</div></div>
  <div class="stat-card"><div class="num amber">${r.queue_generating.toLocaleString()}</div><div class="label">Generating</div></div>
  <div class="stat-card"><div class="num">${r.queue_generated.toLocaleString()}</div><div class="label">Generated</div></div>
  <div class="stat-card"><div class="num green">${r.queue_sent.toLocaleString()}</div><div class="label">Sent</div></div>
  <div class="stat-card"><div class="num red">${r.queue_failed.toLocaleString()}</div><div class="label">Failed</div></div>
</div>
</div>

<div class="section">
<div class="section-header"><h2>📈 Engagement</h2></div>
<div class="stat-grid" style="grid-template-columns:repeat(auto-fill,minmax(90px,1fr))">
  <div class="stat-card"><div class="num">${r.opens.toLocaleString()}</div><div class="label">Total Opens</div></div>
  <div class="stat-card"><div class="num amber">${r.open_pct}%</div><div class="label">Open Rate</div></div>
  <div class="stat-card"><div class="num">${r.clicks.toLocaleString()}</div><div class="label">Total Clicks</div></div>
  <div class="stat-card"><div class="num amber">${r.click_pct}%</div><div class="label">Click Rate</div></div>
  <div class="stat-card"><div class="num">${r.scored.toLocaleString()}</div><div class="label">Scored</div></div>
  <div class="stat-card"><div class="num">${r.researched.toLocaleString()}</div><div class="label">Researched</div></div>
</div>
</div>
</div>

<!-- ── Per-Site Breakdown + Scores (side by side) ── -->
<div style="display:grid;grid-template-columns:1fr 1fr;gap:1.5rem;margin-bottom:2.5rem">
<div class="section">
<div class="section-header"><h2>🌐 Per-Site Breakdown</h2></div>
<div class="table-wrap">
<table>
  <tr><th>Source</th><th class="num-col">Jobs</th></tr>
  ${sites.map(s => `<tr><td>${esc(s.source_site)}</td><td class="num-col">${s.cnt.toLocaleString()}</td></tr>`).join('')}
</table>
</div>
</div>

<div class="section">
<div class="section-header"><h2>📊 Score Distribution</h2></div>
${scores.length > 0 ? `<div class="table-wrap" style="padding:.7rem">
${scores.map(s => {
  const maxCnt = Math.max(...scores.map(x => x.cnt));
  const pct = maxCnt > 0 ? (s.cnt / maxCnt) * 100 : 0;
  const bin = Math.min(4, Math.floor(s.llm_score / 2.5));
  return `<div class="score-row"><span class="score-label">${s.llm_score}/10</span><div class="score-bar-wrap"><div class="score-bar s${bin}" style="width:${pct}%"></div></div><span class="score-count">${s.cnt.toLocaleString()}</span></div>`;
}).join('')}
</div>` : '<div style="color:oklch(0.5 0.02 260);font-size:.82rem">No scored jobs yet</div>'}
</div>
</div>

<!-- ── Run History ── -->
<div class="section">
<div class="section-header"><h2>🔄 Run History</h2></div>
${runs.length > 0 ? `<div class="table-wrap">
<table>
  <tr><th>Workflow</th><th>Mode</th><th class="col-status">Status</th><th class="num-col">Found</th><th class="num-col">Queued</th><th class="num-col">Sent</th><th class="num-col">Failed</th><th>Error</th><th>Started</th></tr>
  ${runs.map(r => {
    const statusClass = r.status === 'completed' ? 'completed' : r.status === 'failed' || r.status === 'error' ? 'failed' : 'running';
    return `<tr>
      <td>${esc(r.workflow)}</td>
      <td>${r.mode || '-'}</td>
      <td><span class="status-badge ${statusClass}">${r.status}</span></td>
      <td class="num-col">${toNum(r.jobs_found).toLocaleString()}</td>
      <td class="num-col">${toNum(r.emails_queued).toLocaleString()}</td>
      <td class="num-col">${toNum(r.emails_sent).toLocaleString()}</td>
      <td class="num-col">${toNum(r.emails_failed).toLocaleString()}</td>
      <td class="wrap">${r.error_msg || ''}</td>
      <td>${r.started}</td>
    </tr>`;
  }).join('')}
</table>
</div>` : '<div style="color:oklch(0.5 0.02 260);font-size:.82rem">No runs yet</div>'}
</div>

${failures.length > 0 ? `<div class="section">
<div class="section-header"><h2>❌ Recent Failures</h2></div>
<div class="table-wrap">
<table><tr><th>Email</th><th>Company</th><th>Error</th><th>When</th></tr>
${failures.map(f => `<tr><td>${esc(f.email_addr)}</td><td>${esc(f.company_name)}</td><td class="wrap">${esc(f.error_msg)}</td><td>${f.when}</td></tr>`).join('')}
</table>
</div>
</div>` : ''}

${clickByUrl.length > 0 ? `<div class="section">
<div class="section-header"><h2>🖱️ Click Breakdown by URL</h2></div>
<div class="table-wrap">
<table><tr><th>URL</th><th class="num-col">Clicks</th></tr>
${clickByUrl.map(c => `<tr><td class="wrap">${esc(c.url)}</td><td class="num-col">${c.cnt.toLocaleString()}</td></tr>`).join('')}
</table>
</div>
</div>` : ''}

<div class="footer">
  <span>jobhunter-tracker</span>
  <a href="/track?e=test">track pixel</a>
  <a href="/health">health</a>
</div>
</body></html>`;

    res.setHeader('Content-Type', 'text/html; charset=utf-8');
    res.status(200).send(html);
  } catch (err) {
    res.status(500).send('Error: ' + err.message);
  }
};

function esc(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}
