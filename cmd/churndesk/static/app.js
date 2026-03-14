// Churndesk frontend — HTMX + Alpine.js glue
// Alpine.js and HTMX are loaded via CDN in the layout template.
// This file provides:
//   1. HTMX configuration (timeouts, error handling)
//   2. Alpine.js components (feed filters, dismiss animations)

document.addEventListener('DOMContentLoaded', function () {
  // Configure HTMX
  document.body.addEventListener('htmx:configRequest', function (evt) {
    evt.detail.timeout = 15000;
  });

  // Show error toast on HTMX request failure
  document.body.addEventListener('htmx:responseError', function (evt) {
    console.error('HTMX error:', evt.detail.xhr.status, evt.detail.xhr.responseText);
  });

  // Animate dismissed items out
  document.body.addEventListener('htmx:afterRequest', function (evt) {
    var trigger = evt.detail.elt;
    if (trigger && trigger.dataset.dismissTarget) {
      var target = document.getElementById(trigger.dataset.dismissTarget);
      if (target) {
        target.classList.add('dismissed');
        setTimeout(function () { target.remove(); }, 300);
      }
    }
  });
});
