(function () {
  var TEAM = [
    {
      name: 'Ahmed Yasser',
      role: 'Started qo as a sandbox idea for Summer CTF — first product version',
      github: 'https://github.com/ahmedYasserM',
      linkedin: 'https://www.linkedin.com/in/ahmedyasser2592'
    },
    {
      name: 'Amna',
      role: 'Contributor',
      github: 'https://github.com/thisisamna',
      linkedin: null
    },
    {
      name: 'Zyad Salah',
      role: 'Contributor',
      github: 'https://github.com/zyad-elkhewekh',
      linkedin: null
    },
    {
      name: 'Mohammed Naser',
      role: 'Contributor',
      github: 'https://github.com/MohammedNaser28',
      linkedin: 'https://www.linkedin.com/in/mohammed-naser-2253a0235/'
    }
  ];

  var ICONS = {
    github: '<svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12"/></svg>',
    linkedin: '<svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M20.447 20.452h-3.554v-5.569c0-1.328-.027-3.037-1.852-3.037-1.853 0-2.136 1.445-2.136 2.939v5.667H9.351V9h3.414v1.561h.046c.477-.9 1.637-1.85 3.37-1.85 3.601 0 4.267 2.37 4.267 5.455v6.286zM5.337 7.433c-1.144 0-2.063-.926-2.063-2.065 0-1.138.92-2.063 2.063-2.063 1.14 0 2.064.925 2.064 2.063 0 1.139-.925 2.065-2.064 2.065zm1.782 13.019H3.555V9h3.564v11.452zM22.225 0H1.771C.792 0 0 .774 0 1.729v20.542C0 23.227.792 24 1.771 24h20.451C23.2 24 24 23.227 24 22.271V1.729C24 .774 23.2 0 22.222 0h.003z"/></svg>'
  };

  var css = [
    '.qo-footer{background:#161b22;border-top:1px solid #30363d;padding:20px 16px;text-align:center;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;flex-shrink:0;}',
    '.qo-footer-title{color:#e6edf3;font-size:13px;font-weight:700;margin-bottom:4px;}',
    '.qo-footer-title span{color:#e3b341;}',
    '.qo-footer-note{color:#8b949e;font-size:11px;margin-bottom:14px;line-height:1.5;}',
    '.qo-footer-team{display:flex;flex-wrap:wrap;justify-content:center;gap:10px 18px;}',
    '.qo-member{display:flex;align-items:center;gap:8px;background:#0f1117;border:1px solid #30363d;border-radius:999px;padding:5px 12px 5px 6px;transition:border-color .2s,transform .2s;}',
    '.qo-member:hover{border-color:#58a6ff;transform:translateY(-1px);}',
    '.qo-avatar{width:24px;height:24px;border-radius:50%;background:linear-gradient(135deg,#58a6ff,#a371f7);color:#fff;font-size:11px;font-weight:700;display:flex;align-items:center;justify-content:center;flex-shrink:0;}',
    '.qo-member-name{color:#e6edf3;font-size:12px;font-weight:600;white-space:nowrap;}',
    '.qo-member-links{display:flex;align-items:center;gap:6px;}',
    '.qo-icon{width:15px;height:15px;color:#8b949e;display:inline-flex;transition:color .2s;}',
    '.qo-icon:hover{color:#58a6ff;}',
    '.qo-icon svg{width:100%;height:100%;}',
    '@media(max-width:480px){.qo-footer-team{flex-direction:column;align-items:center;}}'
  ].join('');

  function initials(name) {
    return name.split(/\s+/).map(function (w) { return w[0]; }).join('').slice(0, 2).toUpperCase();
  }

  function esc(s) {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/"/g, '&quot;');
  }

  var style = document.createElement('style');
  style.textContent = css;
  document.head.appendChild(style);

  var footer = document.createElement('footer');
  footer.className = 'qo-footer';

  var html = '<div class="qo-footer-team">';
  TEAM.forEach(function (m) {
    var links = '<a class="qo-icon" href="' + m.github + '" target="_blank" rel="noopener noreferrer" title="' + esc(m.name) + ' on GitHub" aria-label="' + esc(m.name) + ' GitHub">' + ICONS.github + '</a>';
    links += '<a class="qo-icon" href="' + (m.linkedin || '#') + '"' +
      (m.linkedin ? ' target="_blank" rel="noopener noreferrer"' : ' aria-disabled="true" onclick="return false" style="opacity:.35;cursor:not-allowed;"') +
      ' title="' + esc(m.name) + ' on LinkedIn" aria-label="' + esc(m.name) + ' LinkedIn">' + ICONS.linkedin + '</a>';
    html += '<div class="qo-member">' +
      '<div class="qo-avatar">' + initials(m.name) + '</div>' +
      '<span class="qo-member-name">' + esc(m.name) + '</span>' +
      '<span class="qo-member-links">' + links + '</span>' +
      '</div>';
  });
  html += '</div>';
  html += '<div class="qo-footer-title">Made with <span>&hearts;</span> by the qo team</div>';
  html += '<div class="qo-footer-note">Special thanks to Ahmed Yasser for starting qo as a sandbox idea for Summer CTF and building the first product version.</div>';
  footer.innerHTML = html;

  // Terminal page hosts everything inside #app (fixed-height flex column).
  var host = document.getElementById('app') || document.body;
  host.appendChild(footer);
})();
