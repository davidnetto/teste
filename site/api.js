/* api.js — cliente HTTP, auth, carrinho e helpers compartilhados */

const API_BASE = ''; // relativo (assume mesmo host); troque se necessário

const API = {
    token() { return localStorage.getItem('flor_token') || ''; },
    user() {
        const raw = localStorage.getItem('flor_user');
        return raw ? JSON.parse(raw) : null;
    },
    setSession(token, user) {
        if (token) localStorage.setItem('flor_token', token);
        if (user) localStorage.setItem('flor_user', JSON.stringify(user));
    },
    logout() {
        localStorage.removeItem('flor_token');
        localStorage.removeItem('flor_user');
    },
    isAdmin() {
        const u = API.user();
        return !!(u && u.role === 'admin');
    },
    async request(method, path, body) {
        const headers = { 'Content-Type': 'application/json' };
        const tok = API.token();
        if (tok) headers['Authorization'] = 'Bearer ' + tok;
        const res = await fetch(API_BASE + path, {
            method,
            headers,
            body: body ? JSON.stringify(body) : undefined,
        });
        const ct = res.headers.get('content-type') || '';
        const data = ct.includes('application/json') ? await res.json().catch(() => null) : await res.text();
        if (!res.ok) {
            const err = new Error((data && data.error) || ('HTTP ' + res.status));
            err.status = res.status;
            err.data = data;
            throw err;
        }
        return data;
    },
    get(path)        { return API.request('GET',    path); },
    post(path, body) { return API.request('POST',   path, body); },
    put(path, body)  { return API.request('PUT',    path, body); },
    patch(path, body){ return API.request('PATCH',  path, body); },
    del(path)        { return API.request('DELETE', path); },

    // ==== domínio ====
    async login(email, password) {
        const r = await API.post('/api/auth/login', { email, password });
        API.setSession(r.token, r.user);
        return r;
    },
    async register(payload) {
        const r = await API.post('/api/auth/register', payload);
        API.setSession(r.token, r.user);
        return r;
    },
    me()                 { return API.get('/api/auth/me'); },
    async googleLogin(credential) {
        const r = await API.post('/api/auth/google', { credential });
        API.setSession(r.token, r.user);
        return r;
    },
    googleConfig()       { return API.get('/api/auth/google/config'); },
    cep(cep)             { return API.get('/api/cep?cep=' + encodeURIComponent(cep)); },
    categoryTree()       { return API.get('/api/categories'); },
    categoriesFlat()     { return API.get('/api/categories?flat=1'); },
    products(params={})  {
        const q = new URLSearchParams(params).toString();
        return API.get('/api/products' + (q ? '?' + q : ''));
    },
    product(id)          { return API.get('/api/products/' + encodeURIComponent(id)); },
    createCategory(c)    { return API.post('/api/categories', c); },
    updateCategory(id,c) { return API.put('/api/categories/' + id, c); },
    deleteCategory(id)   { return API.del('/api/categories/' + id); },
    createProduct(p)     { return API.post('/api/products', p); },
    updateProduct(id, p) { return API.put('/api/products/' + id, p); },
    deleteProduct(id)    { return API.del('/api/products/' + id); },
    createOrder(o)       { return API.post('/api/orders', o); },
    getOrder(id)         { return API.get('/api/orders/' + id); },
    paymentStatus(id)    { return API.get('/api/orders/' + id + '/payment-status'); },
    myOrders()           { return API.get('/api/me/orders'); },
    adminOrders()        { return API.get('/api/admin/orders'); },
    markReady(id, msg)   { return API.post('/api/admin/orders/' + id + '/ready', { message: msg || '' }); },
    quote(payload)       { return API.post('/delivery/quotations', payload); },
};

/* ===== Cart ===== */
const Cart = {
    items() { return JSON.parse(localStorage.getItem('flor_cart') || '[]'); },
    save(items) { localStorage.setItem('flor_cart', JSON.stringify(items)); Cart.notify(); },
    add(p) {
        const items = Cart.items();
        const existing = items.find(it => it.product_id === p.id);
        if (existing) { existing.quantity++; }
        else {
            items.push({
                product_id: p.id,
                name: p.name,
                image: (p.images && p.images[0]) || '',
                quantity: 1,
                unit_cents: p.price_cents,
                under_consult: p.price_cents == null,
            });
        }
        Cart.save(items);
    },
    remove(productID) {
        Cart.save(Cart.items().filter(it => it.product_id !== productID));
    },
    setQty(productID, qty) {
        const items = Cart.items();
        const it = items.find(x => x.product_id === productID);
        if (!it) return;
        it.quantity = Math.max(1, qty | 0);
        Cart.save(items);
    },
    clear() { Cart.save([]); },
    count() { return Cart.items().reduce((n, it) => n + it.quantity, 0); },
    subtotalCents() {
        return Cart.items().reduce((sum, it) => {
            if (it.under_consult || it.unit_cents == null) return sum;
            return sum + it.unit_cents * it.quantity;
        }, 0);
    },
    hasConsult() {
        return Cart.items().some(it => it.under_consult || it.unit_cents == null);
    },
    listeners: [],
    onChange(fn) { Cart.listeners.push(fn); fn(); },
    notify() { Cart.listeners.forEach(fn => { try { fn(); } catch(_){} }); },
};

window.addEventListener('storage', e => { if (e.key === 'flor_cart') Cart.notify(); });

/* ===== Helpers ===== */
function formatBRL(cents) {
    if (cents == null) return 'Sob consulta';
    return (cents / 100).toLocaleString('pt-BR', { style: 'currency', currency: 'BRL' });
}
function maskCEP(value) {
    return (value || '').replace(/\D/g, '').slice(0, 8).replace(/^(\d{5})(\d)/, '$1-$2');
}
function maskPhone(value) {
    let v = (value || '').replace(/\D/g, '').slice(0, 11);
    if (v.length > 6)      v = v.replace(/^(\d{2})(\d{5})(\d{0,4}).*/, '($1) $2-$3');
    else if (v.length > 2) v = v.replace(/^(\d{2})(\d{0,5}).*/,        '($1) $2');
    return v;
}
function getQuery(name) {
    return new URLSearchParams(location.search).get(name);
}
function escapeHTML(s) {
    return (s || '').toString().replace(/[&<>"']/g, m => ({
        '&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;'
    })[m]);
}
function requireAuth(adminOnly) {
    const u = API.user();
    if (!u) {
        location.href = 'login.html?next=' + encodeURIComponent(location.pathname + location.search);
        return false;
    }
    if (adminOnly && u.role !== 'admin') {
        alert('Acesso restrito ao administrador.');
        location.href = 'index.html';
        return false;
    }
    return true;
}

/* ===== Header active link sync ===== */
function syncActiveNav() {
    const path = location.pathname;
    document.querySelectorAll('nav a').forEach(a => {
        const href = a.getAttribute('href');
        if (href && (path.endsWith(href) || (href === 'index.html' && (path.endsWith('/') || path.endsWith('index.html'))))) {
            a.classList.add('active');
        } else {
            a.classList.remove('active');
        }
    });
}

/* ===== Header user area render (shared across all pages) ===== */
function renderHeaderUser(areaId) {
    const area = document.getElementById(areaId);
    if (!area) return;
    const u = API.user();
    if (!u) {
        area.innerHTML = `<a href="login.html" style="padding:8px 16px;border-radius:30px;font-size:0.9rem;font-weight:500;color:var(--text-medium);transition:var(--transition);">Entrar</a>`;
        return;
    }
    const isAdmin = u.role === 'admin';
    area.innerHTML = `
        ${isAdmin ? `<a href="admin.html" style="padding:8px 16px;border-radius:30px;font-size:0.85rem;font-weight:600;color:var(--green-dark);background:var(--green-pale);">⚙️ Admin</a>` : `<a href="meus-pedidos.html" style="padding:8px 16px;border-radius:30px;font-size:0.9rem;font-weight:500;color:var(--text-medium);">Meus Pedidos</a>`}
        <button onclick="API.logout(); location.reload();" style="padding:8px 14px;border-radius:30px;border:1px solid #ddd;background:transparent;cursor:pointer;font-size:0.85rem;color:var(--text-medium);">Sair</button>
    `;
}

/* ===== Global cart count badge — runs on EVERY page via turbo:load ===== */
let _cartBadgeRegistered = false;
function _updateCartBadge() {
    const el = document.getElementById('cartCount');
    if (el) el.textContent = Cart.count();
}

document.addEventListener('turbo:load', () => {
    renderHeaderUser('userArea');
    syncActiveNav();
    _updateCartBadge();
    // Register Cart.onChange once; it persists because the header is data-turbo-permanent
    if (!_cartBadgeRegistered) {
        Cart.onChange(_updateCartBadge);
        _cartBadgeRegistered = true;
    }
});

/* ===== toggleCart — safe for pages without cart panel ===== */
function toggleCart() {
    const overlay = document.getElementById('cartOverlay');
    const panel   = document.getElementById('cartPanel');
    if (overlay && panel) {
        overlay.classList.toggle('show');
        panel.classList.toggle('show');
    } else {
        // On pages without the cart panel (e.g. catalogo.html), go to home with cart open
        location.href = 'index.html#carrinho';
    }
}
