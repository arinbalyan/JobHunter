const { Pool } = require('pg');
const pool = new Pool({ connectionString: process.env.DATABASE_URL });

module.exports = async (req, res) => {
  try {
    const r = {};
    const q = async (sql) => {
      const { rows } = await pool.query(sql);
      return rows;
    };

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

    // ── Recent clicks ──
    const clicks = await q("SELECT c.url, to_char(c.clicked_at, 'Mon DD HH24:MI') as when FROM click_log c ORDER BY c.clicked_at DESC LIMIT 10");

    const total_emails = r.queue_pending + r.queue_generating + r.queue_generated + r.queue_sent + r.queue_failed;
    const filtered_approx = Math.max(0, r.total_jobs - r.with_emails);

    // ── HTML ──
    const html = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>JobHunter Dashboard</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,system-ui,sans-serif;background:#f5f5f5;color:#222;padding:1.5rem;max-width:1200px;margin:0 auto}
h1{font-size:1.4rem;margin-bottom:.5rem;color:#111}
.sub{color:#666;font-size:.85rem;margin-bottom:1.5rem}
.section{margin-bottom:2rem}
h2{font-size:1.1rem;margin-bottom:.75rem;color:#333;border-bottom:2px solid #e0e0e0;padding-bottom:.3rem}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(130px,1fr));gap:.75rem;margin-bottom:1.5rem}
.card{background:#fff;border-radius:8px;padding:1rem;box-shadow:0 1px 3px rgba(0,0,0,.08);text-align:center}
.card .num{font-size:1.6rem;font-weight:700;color:#0a7cff}
.card .label{font-size:.75rem;color:#666;margin-top:.15rem}
.card.green .num{color:#16a34a};.card.yellow .num{color:#d97706};.card.red .num{color:#dc2626};.card.purple .num{color:#7c3aed};.card.grey .num{color:#888}
.funnel{display:flex;flex-wrap:wrap;gap:.5rem;align-items:center;margin-bottom:1.5rem}
.funnel .stage{background:#fff;border-radius:6px;padding:.6rem 1rem;box-shadow:0 1px 2px rgba(0,0,0,.08);text-align:center;min-width:80px}
.funnel .stage .num{font-weight:700;font-size:1.1rem}
.funnel .stage .lbl{font-size:.7rem;color:#666}
.funnel .arrow{color:#ccc;font-size:1.2rem}
table{width:100%;border-collapse:collapse;font-size:.82rem;margin-bottom:1.5rem}
th,td{padding:.4rem .6rem;text-align:left;border-bottom:1px solid #eee}
th{background:#fafafa;font-weight:600;color:#555;position:sticky;top:0}
tr:hover{background:#f8f9ff}
.num-col{text-align:right;font-variant-numeric:tabular-nums}
.wrap{max-width:300px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.footer{margin-top:1.5rem;font-size:.75rem;color:#999;text-align:center}
</style></head>
<body>
<h1>📊 JobHunter Pipeline</h1>
<div class="sub">Live stats from NeonDB · last refreshed now</div>

<div class="section">
<h2>Pipeline Funnel</h2>
<div class="funnel">
  <div class="stage"><div class="num" style="color:#0a7cff">${r.total_jobs}</div><div class="lbl">Scraped</div></div>
  <div class="arrow">→</div>
  <div class="stage"><div class="num" style="color:#888">-${filtered_approx}</div><div class="lbl">Filtered</div></div>
  <div class="arrow">→</div>
  <div class="stage"><div class="num" style="color:#16a34a">${r.with_emails}</div><div class="lbl">With Emails</div></div>
  <div class="arrow">→</div>
  <div class="stage"><div class="num" style="color:#7c3aed">${r.scored}</div><div class="lbl">Scored</div></div>
  <div class="arrow">→</div>
  <div class="stage"><div class="num" style="color:#0a7cff">${total_emails}</div><div class="lbl">Queued</div></div>
  <div class="arrow">→</div>
  <div class="stage"><div class="num" style="color:#16a34a">${r.queue_sent}</div><div class="lbl">Sent</div></div>
  <div class="arrow">→</div>
  <div class="stage"><div class="num" style="color:#d97706">${r.unique_opens}</div><div class="lbl">Opened</div></div>
  <div class="arrow">→</div>
  <div class="stage"><div class="num" style="color:#dc2626">${r.unique_clicked}</div><div class="lbl">Clicked</div></div>
</div>
</div>

<div class="section">
<h2>📬 Email Queue</h2>
<div class="grid">
  <div class="card purple"><div class="num">${r.queue_pending}</div><div class="label">Pending</div></div>
  <div class="card yellow"><div class="num">${r.queue_generating}</div><div class="label">Generating</div></div>
  <div class="card" style="border-left:3px solid #0a7cff"><div class="num">${r.queue_generated}</div><div class="label">Generated</div></div>
  <div class="card green"><div class="num">${r.queue_sent}</div><div class="label">Sent</div></div>
  <div class="card red"><div class="num">${r.queue_failed}</div><div class="label">Failed</div></div>
</div>
</div>

<div class="section">
<h2>📈 Engagement</h2>
<div class="grid">
  <div class="card green"><div class="num">${r.opens}</div><div class="label">Total Opens</div></div>
  <div class="card yellow"><div class="num">${r.open_pct}%</div><div class="label">Open Rate</div></div>
  <div class="card purple"><div class="num">${r.clicks}</div><div class="label">Total Clicks</div></div>
  <div class="card" style="border-left:3px solid #0a7cff"><div class="num">${r.click_pct}%</div><div class="label">Click Rate</div></div>
  <div class="card green"><div class="num">${r.scored}</div><div class="label">Scored Jobs</div></div>
  <div class="card purple"><div class="num">${r.researched}</div><div class="label">Researched</div></div>
</div>
</div>

<div class="section">
<h2>🌐 Per-Site Breakdown</h2>
<table><tr><th>Source</th><th class="num-col">Jobs</th></tr>
${sites.map(s => `<tr><td>${esc(s.source_site)}</td><td class="num-col">${s.cnt}</td></tr>`).join('')}
</table>
</div>

<div class="section">
<h2>📊 Score Distribution</h2>
<table><tr><th>Score</th><th class="num-col">Count</th></tr>
${scores.map(s => `<tr><td>${s.llm_score}/10</td><td class="num-col">${s.cnt}</td></tr>`).join('')}
</table>
</div>

<div class="section">
<h2>🔄 Run History</h2>
<table><tr><th>Workflow</th><th>Mode</th><th>Status</th><th class="num-col">Found</th><th class="num-col">Queued</th><th class="num-col">Sent</th><th class="num-col">Failed</th><th>Error</th><th>Started</th></tr>
${runs.map(r => `<tr>
  <td>${esc(r.workflow)}</td>
  <td>${r.mode || '-'}</td>
  <td>${r.status}</td>
  <td class="num-col">${r.jobs_found}</td>
  <td class="num-col">${r.emails_queued}</td>
  <td class="num-col">${r.emails_sent}</td>
  <td class="num-col">${r.emails_failed}</td>
  <td class="wrap">${r.error_msg || ''}</td>
  <td>${r.started}</td>
</tr>`).join('')}
</table>
</div>

${failures.length > 0 ? `<div class="section">
<h2>❌ Recent Failures</h2>
<table><tr><th>Email</th><th>Company</th><th>Error</th><th>When</th></tr>
${failures.map(f => `<tr><td>${esc(f.email_addr)}</td><td>${esc(f.company_name)}</td><td class="wrap">${esc(f.error_msg)}</td><td>${f.when}</td></tr>`).join('')}
</table>
</div>` : ''}

${clicks.length > 0 ? `<div class="section">
<h2>🖱️ Recent Clicks</h2>
<table><tr><th>URL</th><th>When</th></tr>
${clicks.map(c => `<tr><td class="wrap">${esc(c.url)}</td><td>${c.when}</td></tr>`).join('')}
</table>
</div>` : ''}

<div class="footer">jobhunter-tracker · <a href="/track?e=test">track pixel</a> · <a href="/health">health</a></div>
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
