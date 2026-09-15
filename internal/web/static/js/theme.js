// Runs in <head>, before the first paint: reads the theme the person chose,
// or the one their system prefers, and stamps it on <html>. Everything else
// about the theme lives in app.js; this only exists so the page never
// flashes the wrong colours while the rest loads. Paper is the default.
//
// A ?theme=light or ?theme=dark in the address wins for that page load and
// is not remembered: it lets a link, a screenshot or an embed ask for one
// look without touching the person's choice.
(function () {
  try {
    var forced = new URLSearchParams(window.location.search).get('theme');
    var stored = localStorage.getItem('lk-theme');
    var theme = forced === 'light' || forced === 'dark'
      ? forced
      : (stored === 'light' || stored === 'dark'
        ? stored
        : (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'));
    document.documentElement.setAttribute('data-theme', theme);
  } catch (e) {
    /* private mode or storage disabled: the default in the markup stands */
  }
})();
