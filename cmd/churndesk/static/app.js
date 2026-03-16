// Churndesk frontend — HTMX + Alpine.js + GSAP glue
// Alpine.js, GSAP, and HTMX are loaded via CDN in the layout template.
// This file provides:
//   1. Alpine.js data function (toasts, HX-Trigger handling)
//   2. HTMX configuration (timeouts, error handling, dismiss animations)
//   3. GSAP entrance animations (header, feed items, toasts)

// ---------------------------------------------------------------------------
// Alpine.js data function — MUST be global so <body x-data="churndesk()"> can
// find it. Alpine is loaded with `defer`, which runs after DOM parse but before
// DOMContentLoaded. Declaring at the top level ensures availability.
// ---------------------------------------------------------------------------
function churndesk() {
  return {
    toasts: [],
    toastId: 0,

    init() {
      // Listen for HX-Trigger headers from HTMX responses to surface toasts.
      document.addEventListener('htmx:afterRequest', (evt) => {
        const xhr = evt.detail.xhr;
        if (!xhr) return;

        const trigger = xhr.getResponseHeader('HX-Trigger');
        if (!trigger) return;

        try {
          const data = JSON.parse(trigger);
          if (data.newItems) {
            this.addToast(
              `${data.newItems} new item${data.newItems > 1 ? 's' : ''} arrived`,
              'new-items'
            );
          }
          if (data.syncError) {
            this.addToast(data.syncError, 'error');
          }
          if (data.saved) {
            this.addToast('Settings saved', 'success');
          }
        } catch (_e) {
          // HX-Trigger may be a plain string event name, not JSON — ignore.
        }
      });
    },

    addToast(message, type) {
      const id = ++this.toastId;
      this.toasts.push({ id, message, type });

      // Keep at most 4 visible toasts.
      if (this.toasts.length > 4) {
        this.toasts.shift();
      }

      // Animate toast entrance via GSAP after Alpine renders the element.
      this.$nextTick(() => {
        const el = document.querySelector(`[data-toast="${id}"]`);
        if (el && typeof gsap !== 'undefined') {
          gsap.from(el, {
            y: -30,
            scale: 0.85,
            opacity: 0,
            duration: 0.5,
            ease: 'back.out(1.5)',
          });
        }
      });

      // Auto-dismiss after 4.4 seconds.
      setTimeout(() => {
        this.toasts = this.toasts.filter((t) => t.id !== id);
      }, 4400);
    },
  };
}

// ---------------------------------------------------------------------------
// HTMX configuration and GSAP page-load animations — run after DOM is ready.
// ---------------------------------------------------------------------------
document.addEventListener('DOMContentLoaded', function () {
  // --- HTMX configuration ---------------------------------------------------

  // Extend default request timeout.
  document.body.addEventListener('htmx:configRequest', function (evt) {
    evt.detail.timeout = 15000;
  });

  // Log HTMX request failures.
  document.body.addEventListener('htmx:responseError', function (evt) {
    console.error('HTMX error:', evt.detail.xhr.status, evt.detail.xhr.responseText);
  });

  // Animate dismissed items out (CSS class drives the transition, then remove).
  document.body.addEventListener('htmx:afterRequest', function (evt) {
    var trigger = evt.detail.elt;
    if (trigger && trigger.dataset.dismissTarget) {
      var target = document.getElementById(trigger.dataset.dismissTarget);
      if (target) {
        target.classList.add('dismissed');
        setTimeout(function () {
          target.remove();
        }, 300);
      }
    }
  });

  // --- GSAP page-load entrance animations ------------------------------------

  if (typeof gsap !== 'undefined') {
    // Header slides down into place.
    gsap.from('.header', {
      y: -50,
      opacity: 0,
      duration: 0.7,
      ease: 'power3.out',
    });

    // Feed items stagger in.
    gsap.from('.feed-item', {
      y: 30,
      scale: 0.97,
      opacity: 0,
      duration: 0.5,
      stagger: 0.07,
      ease: 'power3.out',
    });
  }

  // --- GSAP animations for new feed items inserted via HTMX ------------------

  document.body.addEventListener('htmx:afterSwap', function (evt) {
    if (typeof gsap === 'undefined') return;

    var target = evt.detail.target;
    if (target && target.id === 'feed') {
      // Animate all feed items in the swapped container — newly inserted items
      // will get the entrance animation while already-visible ones are at their
      // natural position so `from` is effectively a no-op for them.
      gsap.from('#feed .feed-item', {
        y: 30,
        scale: 0.97,
        opacity: 0,
        duration: 0.5,
        stagger: 0.07,
        ease: 'power3.out',
      });
    }
  });
});
