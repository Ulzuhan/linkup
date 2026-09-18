// Runs in <head>, before the first paint, and keeps the same contract as the
// rest of the family: localStorage.theme is "dark", "light" or "system", dark
// is the default, and the resolved value is a class on <html> plus
// color-scheme. External and tiny, because the policy allows no inline
// script and a flash of the wrong theme is exactly what a late script gives.
//
// A ?theme=light or ?theme=dark in the address wins for that page load and is
// not remembered: a link, a screenshot or an embed can ask for one look
// without touching the person's choice.
(function () {
  var theme = "dark";
  try {
    var forced = new URLSearchParams(window.location.search).get("theme");
    var stored = localStorage.getItem("theme") || "dark";
    theme = forced === "light" || forced === "dark" ? forced : stored;
    if (theme === "system") {
      theme = window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
    }
    if (theme !== "light") theme = "dark";
  } catch (e) {
    theme = "dark";
  }
  var root = document.documentElement;
  root.classList.remove("light", "dark");
  root.classList.add(theme);
  root.style.colorScheme = theme;
})();
