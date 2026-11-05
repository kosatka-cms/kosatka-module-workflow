// Editorial workflow module (entity.Module.JS), loaded after app.js on the
// pages that attach it ({{attach}}: an item's form, the workflow's pages).

// ===== Editing lock =====
// An item's form locked by another editor ([data-workflow-locked] over
// it) is read-only: its fields can't be changed, only read and copied,
// and it isn't sent. The form holding the user's lock
// ([data-workflow-lock] - its URL) keeps it while they are active -
// typing, clicking, changing the text: a heartbeat every few minutes
// after activity (none - the lock lapses after data-workflow-lock-minutes)
// tells whether it is still theirs; taken over, the form says who has
// it. Closing the form unsaved releases it; saving does on the server.
(function () {
    var form = document.querySelector('form.node-form');
    if (!form) {
        return;
    }
    var locked = document.querySelector('[data-workflow-locked]');
    if (locked) {
        readOnly(form);
        return;
    }
    var card = form.querySelector('[data-workflow-lock]');
    if (card) {
        keepLock(form, card);
    }

    // readOnly makes form's fields read-only: text ones readOnly (they can
    // still be selected and copied), the rest disabled, its text editors in
    // their read-only mode; submitting does nothing.
    function readOnly(form) {
        form.classList.add('workflow-readonly');
        Array.prototype.forEach.call(form.elements, function (el) {
            var type = (el.type || '').toLowerCase();
            if (type === 'hidden') {
                return;
            }
            if ((el.tagName === 'INPUT' && /^(text|search|url|email|tel|number|date|datetime-local|time)$/.test(type)) || el.tagName === 'TEXTAREA') {
                el.readOnly = true;
            } else {
                el.disabled = true;
            }
        });
        form.addEventListener('submit', function (e) { e.preventDefault(); });
        // TinyMCE loads on first use: its editors, as they start.
        var tries = 0;
        var timer = setInterval(function () {
            var mce = window.tinymce;
            if (mce) {
                clearInterval(timer);
                mce.get().forEach(editorReadOnly);
                mce.on('AddEditor', function (e) { editorReadOnly(e.editor); });
            } else if (++tries > 50) {
                clearInterval(timer);
            }
        }, 200);
    }

    function editorReadOnly(editor) {
        if (editor.initialized) {
            editor.mode.set('readonly');
        } else {
            editor.on('init', function () { editor.mode.set('readonly'); });
        }
    }

    // keepLock sends the heartbeat of card's lock while the user is active
    // and releases it when the form closes unsaved.
    function keepLock(form, card) {
        var url = card.getAttribute('data-workflow-lock');
        var minutes = parseInt(card.getAttribute('data-workflow-lock-minutes'), 10) || 30;
        var every = Math.min(5, Math.max(1, minutes / 3)) * 60000;
        var active = false;
        var lost = false;
        var submitting = false;
        var last = snapshot();
        ['input', 'change', 'keydown', 'pointerdown'].forEach(function (type) {
            form.addEventListener(type, function () { active = true; }, true);
        });
        form.addEventListener('submit', function () { submitting = true; });
        setInterval(function () {
            // Typing in a text editor (its own frame) changes the form.
            var now = snapshot();
            if (now !== last) {
                active = true;
                last = now;
            }
            if (!active || lost) {
                return;
            }
            active = false;
            fetch(url, {method: 'POST', credentials: 'same-origin'})
                .then(function (r) { return r.ok ? r.json() : null; })
                .then(function (d) {
                    if (d && !d.mine) {
                        lost = true;
                        showLost(card.getAttribute('data-workflow-lock-lost').replace('@name', d.who || ''));
                    }
                })
                .catch(function () {});
        }, every);
        window.addEventListener('pagehide', function () {
            if (!submitting && !lost) {
                fetch(url + '/release', {method: 'POST', credentials: 'same-origin', keepalive: true}).catch(function () {});
            }
        });
    }

    // snapshot - the form's text editors' content (what the form's own
    // events don't show).
    function snapshot() {
        var mce = window.tinymce;
        if (!mce) {
            return '';
        }
        return mce.get().map(function (ed) { return ed.id + ':' + ed.getContent(); }).join('\n');
    }

    function showLost(text) {
        var alert = document.createElement('div');
        alert.className = 'alert alert-warning';
        alert.setAttribute('role', 'alert');
        alert.textContent = text;
        form.parentNode.insertBefore(alert, form);
        alert.scrollIntoView({block: 'nearest'});
    }
})();
