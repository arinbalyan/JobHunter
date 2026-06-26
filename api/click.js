const { Pool } = require('pg');
const pool = new Pool({ connectionString: process.env.DATABASE_URL });

module.exports = async (req, res) => {
  const emailId = req.query.e;
  if (!emailId) return res.status(400).end('missing e');
  try {
    await pool.query('UPDATE tracking SET clicks = clicks + 1, last_clicked_at = now() WHERE email_id = $1', [emailId]);
  } catch (_) {}
  res.redirect(302, 'https://linkedin.com/in/arinbalyan');
};
