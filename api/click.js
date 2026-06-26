const { Pool } = require('pg');
const pool = new Pool({ connectionString: process.env.DATABASE_URL });

module.exports = async (req, res) => {
  const emailId = req.query.e;
  const targetUrl = req.query.url || 'https://linkedin.com/in/arinbalyan';

  if (!emailId) return res.status(400).end('missing e');

  try {
    await pool.query(
      "UPDATE tracking SET clicks = clicks + 1, last_clicked_at = now() WHERE email_id = $1",
      [emailId]
    );
    await pool.query(
      "INSERT INTO click_log (email_id, url) VALUES ($1, $2)",
      [emailId, targetUrl]
    );
  } catch (_) {}

  res.redirect(302, targetUrl);
};
