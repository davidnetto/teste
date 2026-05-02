"""
Gera site/catalogo.html com o design solicitado pelo usuário, 
integrando com o sistema de navbar/carrinho (api.js e turbo).
"""
import json, re

INPUT  = "data/produtos_enriquecidos.json"
OUTPUT = "site/catalogo.html"

# --- Normalizações e Helpers ---

def terra_cat(tipo):
    if not tipo: return "drenada"
    t = tipo.lower()
    # Simplificação de categorias de terra para os chips do novo design
    if any(x in t for x in ["substrato", "pinus", "musgo"]): return "substrato"
    if "argil" in t: return "argilosa"
    if any(x in t for x in ["organica", "materia", "fertil"]): return "organica"
    return "drenada"

def calc_dificuldade(p):
    # Lógica de dificuldade para os chips do novo design
    umidade = p.get("umidade", "").lower()
    luz = p.get("luz", [])
    
    # Critérios simplificados baseados no estilo sugerido
    if "alta" in umidade or len(luz) == 1:
        return "Avancado"
    if "mǸdia" in umidade or "media" in umidade:
        return "Moderado"
    return "Facil"

CAT_EMOJI = {
    'Plantas de Interior': '🪴',
    'Flores': '🌸',
    'Arvores': '🌳',
    'Árvores': '🌳',
    'Palmeiras': '🌴',
    'Suculentas e Cactos': '🌵',
    'Gramineas e Bambus': '🎋',
    'Gramíneas e Bambus': '🎋',
    'Trepadeiras': '🌿',
    'Samambaias e Fetos': '🌾',
    'Ervas e Temperos': '🌿',
    'Aquaticas': '🌊',
    'Kokedamas': '🪴'
}

# --- Carregamento de Dados ---

try:
    with open(INPUT, encoding="utf-8") as f:
        data = json.load(f)
        # Se for um objeto com "Products", pega a lista
        plantas_raw = data.get("Products", data) if isinstance(data, dict) else data
except Exception as e:
    print(f"Erro ao ler {INPUT}: {e}")
    plantas_raw = []

plantas = []
for p in plantas_raw:
    u_raw = p.get("umidade", "").replace("mǸdia", "média")
    
    # Normalização de umidade para o JS (baixa, media, alta)
    u_norm = "media"
    if "baixa" in u_raw.lower(): u_norm = "baixa"
    elif "alta" in u_raw.lower(): u_norm = "alta"

    p_proc = {
        "id": p.get("url", "").split('/')[-1] or p.get("nome", "").replace(" ", "-"),
        "nome": p.get("nome", "Sem Nome"),
        "preco_atual": p.get("preco_atual", 0),
        "preco_original": p.get("preco_original", 0),
        "categoria": p.get("categoria", "Outros"),
        "subcategoria": p.get("subcategoria", ""),
        "luz": p.get("luz", []),
        "umidade": u_raw, # Texto original
        "umid_key": u_norm, # Chave para filtro
        "tipo_terra": p.get("tipo_terra", ""),
        "terra_key": terra_cat(p.get("tipo_terra", "")),
        "descricao": p.get("descricao", ""),
        "imagens": p.get("imagens", []),
        "dificuldade": calc_dificuldade(p)
    }
    plantas.append(p_proc)

plantas_js = json.dumps(plantas, ensure_ascii=False)

# --- Template HTML ---

HTML = f"""<!DOCTYPE html>
<html lang="pt-BR">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>🌿 Catalogo de Plantas — Flor Em Tina</title>
    <script src="https://cdn.jsdelivr.net/npm/@hotwired/turbo@7.3.0/dist/turbo.es2017-umd.js"></script>
    <link rel="stylesheet" href="styles.css">
    <style>
        *,*::before,*::after{{box-sizing:border-box;margin:0;padding:0}}
        :root{{
          --g-dark:#1b4332;--g-mid:#2d6a4f;--g-light:#52b788;--g-pale:#d8f3dc;
          --cream:#f8f4ef;--card:#ffffff;
          --shadow:0 2px 16px rgba(27,67,50,.10);--shadow-h:0 8px 32px rgba(27,67,50,.18);
          --r:16px;
        }}
        
        /* Ajuste para o body não quebrar com a navbar do projeto */
        body{{font-family:'Segoe UI',system-ui,sans-serif;background:var(--cream);color:#1a2e1a;min-height:100vh; padding-top: 0;}}

        /* HERO */
        .hero{{background:linear-gradient(135deg,var(--g-dark) 0%,var(--g-mid) 60%,#40916c 100%);
          color:#fff;padding:60px 24px 40px;text-align:center;position:relative;overflow:hidden}}
        .hero::before{{content:'🌿🌱🌺🌻🌴🌵🍃🪴';position:absolute;top:-10px;left:0;right:0;
          font-size:60px;opacity:.07;letter-spacing:8px;white-space:nowrap;overflow:hidden;pointer-events:none}}
        .hero h1{{font-size:clamp(1.6rem,4vw,2.4rem);font-weight:800;letter-spacing:-1px; margin: 0;}}
        .hero p{{margin-top:8px;opacity:.8;font-size:.95rem}}
        .hero-badge{{display:inline-block;margin-top:14px;background:rgba(255,255,255,.18);
          border:1px solid rgba(255,255,255,.3);border-radius:999px;padding:4px 16px;
          font-size:.85rem;backdrop-filter:blur(8px)}}

        /* LEGENDA */
        .legend{{background:var(--g-dark);color:rgba(255,255,255,.85);
          display:flex;flex-wrap:wrap;justify-content:center;gap:24px;padding:12px 24px;font-size:.78rem}}
        .legend-g{{display:flex;align-items:center;gap:6px}}
        .legend-t{{font-weight:700;opacity:.55;margin-right:2px;text-transform:uppercase;font-size:.65rem;letter-spacing:.5px}}
        .legend-i{{opacity:.85}}

        /* HEADER E BUSCA */
        .filters-top{{display:flex;gap:10px;align-items:center;width:100%;max-width:1400px;margin:0 auto;padding:16px 24px;background:#fff;position:sticky;top:0;z-index:10;box-shadow:0 2px 8px rgba(0,0,0,.06)}}
        .search-w{{flex:1}}
        .search-w input{{width:100%;padding:10px 16px 10px 38px;border-radius:12px;
          border:1px solid #e0e0e0;font-size:.9rem;outline:none;transition:border-color .15s;
          background:#f9f9f9 url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' fill='%23999' viewBox='0 0 16 16'%3E%3Cpath d='M11.742 10.344a6.5 6.5 0 1 0-1.397 1.398h-.001l3.85 3.85a1 1 0 0 0 1.415-1.414l-3.85-3.85a1.007 1.007 0 0 0-.115-.099zm-5.242 1.656a5.5 5.5 0 1 1 0-11 5.5 5.5 0 0 1 0 11z'/%3E%3C/svg%3E") no-repeat 14px center}}
        .search-w input:focus{{border-color:var(--g-light);background-color:#fff}}
        .btn-toggle-filters{{padding:10px 16px;border-radius:12px;border:1px solid #e0e0e0;background:#fff;font-size:.9rem;font-weight:600;color:var(--g-dark);cursor:pointer;white-space:nowrap;display:flex;align-items:center;gap:6px;transition:background .2s}}
        .btn-toggle-filters:hover{{background:var(--g-pale);border-color:var(--g-light)}}

        /* MODAL DE FILTROS */
        .filter-modal-overlay{{position:fixed;inset:0;background:rgba(0,0,0,0.5);z-index:1000;display:none;opacity:0;transition:opacity 0.2s;}}
        .filter-modal-overlay.open{{display:block;opacity:1;}}
        .filter-modal{{position:absolute;top:0;right:0;bottom:0;width:100%;max-width:400px;background:#fff;display:flex;flex-direction:column;transform:translateX(100%);transition:transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);box-shadow:-4px 0 24px rgba(0,0,0,0.1)}}
        .filter-modal-overlay.open .filter-modal{{transform:translateX(0)}}
        
        .fm-header{{padding:16px 20px;border-bottom:1px solid #eee;display:flex;justify-content:space-between;align-items:center;}}
        .fm-header h3{{font-size:1.1rem;margin:0;color:var(--g-dark);}}
        .fm-close{{background:none;border:none;font-size:1.2rem;cursor:pointer;color:#999;padding:4px;}}
        
        .fm-body{{flex:1;overflow-y:auto;padding:20px;display:flex;flex-direction:column;gap:24px;}}
        .fgroup{{display:flex;flex-direction:column;gap:10px;}}
        .flabel{{font-size:.8rem;font-weight:700;text-transform:uppercase;letter-spacing:1px;color:#aaa;}}
        .chips-grid{{display:flex;flex-wrap:wrap;gap:8px;}}
        
        .chip{{display:inline-flex;align-items:center;padding:8px 14px;border-radius:8px;
          font-size:.85rem;border:1px solid #e0e0e0;background:#fafafa;cursor:pointer;
          transition:all .1s;user-select:none;font-weight:500;color:#555;}}
        .chip:active{{transform:scale(0.96)}}
        .chip.active{{background:#e8f5e9;color:var(--g-dark);border-color:var(--g-light);font-weight:600}}
        .chip.active::before{{content:'✔';margin-right:6px;font-size:.7rem;}}
        
        .fm-footer{{padding:16px 20px;border-top:1px solid #eee;background:#fff;display:flex;gap:12px;}}
        .btn-clear{{flex:1;padding:12px;border-radius:8px;font-size:.9rem;font-weight:600;border:1px solid #e0e0e0;background:#fff;cursor:pointer;color:#555;}}
        .btn-apply{{flex:2;padding:12px;border-radius:8px;font-size:.9rem;font-weight:700;border:none;background:var(--g-dark);color:#fff;cursor:pointer;transition:background .2s;}}
        .btn-apply:hover{{background:var(--g-mid)}}

        .active-filters-bar{{display:flex;gap:8px;flex-wrap:wrap;padding:0 24px 12px;max-width:1400px;margin:0 auto;}}
        .active-chip{{display:inline-flex;align-items:center;gap:6px;padding:4px 10px;background:#e8f5e9;color:var(--g-dark);border-radius:16px;font-size:.75rem;font-weight:600;}}
        .active-chip button{{background:none;border:none;color:inherit;cursor:pointer;font-size:.8rem;}}

        /* STATS */
        .stats{{max-width:1400px;margin:0 auto;padding:12px 24px;
          display:flex;align-items:center;gap:10px;font-size:.82rem;color:#777;flex-wrap:wrap}}
        .stats strong{{color:var(--g-dark);font-size:.95rem}}

        /* GRID */
        .grid{{max-width:1400px;margin:0 auto;
          display:grid;grid-template-columns:repeat(auto-fill,minmax(250px,1fr));
          gap:18px;padding:0 24px 48px}}

        /* CARD */
        .card{{background:var(--card);border-radius:12px;box-shadow:0 4px 12px rgba(0,0,0,.04);
          overflow:hidden;transition:transform .2s,box-shadow .2s;border:1px solid #f0f0f0;
          display:flex;flex-direction:column;cursor:pointer;position:relative;}}
        .card:hover{{transform:translateY(-2px);box-shadow:0 8px 24px rgba(0,0,0,.08)}}

        .btn-fav{{position:absolute;top:10px;left:10px;z-index:5;background:#fff;border:none;border-radius:50%;width:32px;height:32px;display:flex;align-items:center;justify-content:center;font-size:1.1rem;box-shadow:0 2px 6px rgba(0,0,0,.1);cursor:pointer;transition:transform 0.15s;}}
        .btn-fav:active{{transform:scale(0.85)}}
        
        .b-disc{{position:absolute;top:10px;right:10px;z-index:5;background:#ef5350;color:#fff;
          padding:4px 8px;border-radius:6px;font-size:.7rem;font-weight:800;letter-spacing:0.5px;}}

        .cimg{{width:100%;aspect-ratio:1;background:#f5f5f5;
          display:flex;align-items:center;justify-content:center;font-size:3.2rem;
          position:relative;overflow:hidden;flex-shrink:0}}
        .cimg img{{position:absolute;inset:0;width:100%;height:100%;object-fit:cover;
          opacity:0;transition:opacity .3s,transform .4s}}
        .cimg img.loaded{{opacity:1}}
        .card:hover .cimg img{{transform:scale(1.05)}}
        .cimg-em{{position:relative;z-index:1;opacity:0.3}}

        .cbody{{padding:12px;flex:1;display:flex;flex-direction:column;gap:6px;}}
        .cname{{font-size:.95rem;font-weight:700;color:#111;line-height:1.2;margin-bottom:2px;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden;}}
        .csub{{font-size:.75rem;color:#777;margin-bottom:4px;}}
        
        .price-row{{display:flex;align-items:baseline;gap:6px;margin-top:auto;}}
        .price-c{{font-size:1.2rem;font-weight:800;color:var(--g-dark)}}
        .price-o{{font-size:.8rem;color:#999;text-decoration:line-through}}

        .card-tech{{display:flex;gap:12px;font-size:.8rem;color:#555;margin-top:6px;margin-bottom:12px;font-weight:500;}}
        .card-tech span{{display:flex;align-items:center;gap:4px;}}
        
        .btn-add-card{{width:100%;padding:10px;background:#f5f5f5;color:var(--g-dark);border:1px solid #e0e0e0;border-radius:8px;font-weight:700;font-size:.85rem;cursor:pointer;transition:all 0.2s;}}
        .btn-add-card:hover{{background:var(--g-pale);border-color:var(--g-light);color:var(--g-dark);}}
        .btn-add-card.added{{background:var(--g-dark);color:#fff;border-color:var(--g-dark);}}

        /* EMPTY */
        .empty{{grid-column:1/-1;text-align:center;padding:60px 20px;color:#aaa}}
        .empty p{{font-size:3rem;margin-bottom:10px}}

        /* MODAL */
        .overlay{{display:none;position:fixed;inset:0;background:rgba(0,0,0,.55);z-index:500;
          align-items:center;justify-content:center;padding:20px;backdrop-filter:blur(4px)}}
        .overlay.open{{display:flex}}
        .modal{{background:#fff;border-radius:20px;max-width:540px;width:100%;max-height:90vh;
          overflow-y:auto;box-shadow:0 24px 60px rgba(0,0,0,.3);animation:min .2s ease}}
        @keyframes min{{from{{opacity:0;transform:scale(.95) translateY(16px)}}}}
        .mimg{{width:100%;aspect-ratio:16/9;background:linear-gradient(135deg,#c8e6c9,#a5d6a7);
          display:flex;align-items:center;justify-content:center;font-size:5rem;
          border-radius:20px 20px 0 0;position:relative;overflow:hidden}}
        .mimg img{{position:absolute;inset:0;width:100%;height:100%;object-fit:cover}}
        .mclose{{position:absolute;top:12px;right:12px;background:rgba(0,0,0,.45);color:#fff;
          border:none;border-radius:50%;width:32px;height:32px;font-size:1rem;cursor:pointer;
          display:flex;align-items:center;justify-content:center;backdrop-filter:blur(4px);z-index:2}}
        .mbody{{padding:20px 22px 26px}}
        .mcat {{font-size:.7rem;font-weight:700;text-transform:uppercase;color:var(--g-light);letter-spacing:.5px}}
        .mname{{font-size:1.4rem;font-weight:800;color:var(--g-dark);margin:3px 0 2px}}
        .msub {{color:#aaa;font-size:.82rem}}
        .mpr  {{display:flex;align-items:baseline;gap:9px;margin:10px 0}}
        .mpr-c{{font-size:1.5rem;font-weight:800;color:var(--g-dark)}}
        .mpr-o{{font-size:.86rem;color:#bbb;text-decoration:line-through}}
        .mpr-d{{background:#ef5350;color:#fff;padding:2px 7px;border-radius:6px;font-size:.73rem;font-weight:800}}
        .mattrs{{display:flex;flex-direction:column;gap:9px;margin:14px 0}}
        .mattr{{display:flex;align-items:center;gap:10px}}
        .mal  {{width:64px;font-size:.68rem;font-weight:700;text-transform:uppercase;color:#bbb;flex-shrink:0}}
        .mdesc{{font-size:.8rem;color:#666;line-height:1.6;margin-top:12px;max-height:130px;overflow-y:auto}}
        .mbtn{{display:block;width:100%;padding:13px;margin-top:16px;background:var(--g-dark);color:#fff;
          border:none;border-radius:11px;font-size:.95rem;font-weight:700;cursor:pointer;transition:background .15s}}
        .mbtn:hover{{background:var(--g-mid)}}

        @media(max-width:600px){{
          .legend{{display:none}}
          .grid{{grid-template-columns:repeat(auto-fill,minmax(150px,1fr));gap:12px;padding:0 16px 32px}}
          .cname{{font-size:.85rem}}
          .filter-modal{{max-width:100%;}}
        }}
    </style>
</head>
<body>

<!-- NAVBAR DO PROJETO -->
<header id="main-header" data-turbo-permanent>
    <div class="header-inner">
        <a href="index.html" class="logo">
            <div class="logo-icon">🌸</div>
            <div class="logo-text"><h1>Flor Em Tina</h1><span>Plantas & Jardim</span></div>
        </a>
        <nav>
            <a href="index.html">Início</a>
            <a href="catalogo.html" class="active">Catálogo Completo</a>
            <a href="index.html#entrega">Entrega</a>
            <span id="userArea" style="display: flex; gap: 8px; align-items: center;"></span>
            <button class="cart-btn" onclick="toggleCart()">🛒 <span class="cart-count" id="cartCount">0</span></button>
        </nav>
    </div>
</header>

<div id="spa-content">
    <div class="hero">
      <h1>🌿 Catalogo de Plantas</h1>
      <p>Encontre a planta perfeita para o seu espaco</p>
      <div class="hero-badge" id="hbadge">Carregando...</div>
    </div>

    <div class="legend">
      <div class="legend-g"><span class="legend-t">Luz</span><span class="legend-i">🌑 Sombra</span><span class="legend-i">🌤️ Meia-luz</span><span class="legend-i">☀️ Sol pleno</span></div>
      <div class="legend-g"><span class="legend-t">Agua</span><span class="legend-i">💧 Baixa</span><span class="legend-i">💧💧 Media</span><span class="legend-i">💧💧💧 Alta</span></div>
      <div class="legend-g"><span class="legend-t">Terra</span><span class="legend-i">🏖️ Drenada</span><span class="legend-i">🌱 Organica</span><span class="legend-i">🟫 Argilosa</span><span class="legend-i">🪴 Substrato</span></div>
      <div class="legend-g"><span class="legend-t">Cuidado</span><span class="legend-i">🟢 Facil</span><span class="legend-i">🟡 Moderado</span><span class="legend-i">🔴 Avancado</span></div>
    </div>

    <div class="filters-top">
      <div class="search-w"><input type="text" id="search" placeholder="Buscar plantas..." oninput="af()"></div>
      <button class="btn-toggle-filters" onclick="toggleFilterModal()">⚙️ Filtros</button>
    </div>

    <div class="active-filters-bar" id="activeFiltersBar" style="display:none;"></div>

    <div class="stats">
      <span><strong id="vcount">0</strong> plantas encontradas</span>
    </div>

    <div class="grid" id="grid"></div>

    <!-- FILTER MODAL -->
    <div class="filter-modal-overlay" id="filterModalOverlay" onclick="toggleFilterModal(event)">
      <div class="filter-modal" id="filterModal">
        <div class="fm-header">
          <h3>Filtros</h3>
          <button class="fm-close" onclick="toggleFilterModal({{force:true}})">✕</button>
        </div>
        <div class="fm-body">
          <div class="fgroup">
            <span class="flabel">🌱 Categoria</span>
            <div class="chips-grid">
              <span class="chip" data-f="cat" data-v="Plantas de Interior" onclick="tc(this)">Interior</span>
              <span class="chip" data-f="cat" data-v="Flores" onclick="tc(this)">Flores</span>
              <span class="chip" data-f="cat" data-v="Arvores" onclick="tc(this)">Árvores</span>
              <span class="chip" data-f="cat" data-v="Palmeiras" onclick="tc(this)">Palmeiras</span>
              <span class="chip" data-f="cat" data-v="Suculentas e Cactos" onclick="tc(this)">Suculentas</span>
              <span class="chip" data-f="cat" data-v="Gramineas e Bambus" onclick="tc(this)">Bambus</span>
              <span class="chip" data-f="cat" data-v="Trepadeiras" onclick="tc(this)">Trepadeiras</span>
              <span class="chip" data-f="cat" data-v="Samambaias e Fetos" onclick="tc(this)">Samambaias</span>
              <span class="chip" data-f="cat" data-v="Ervas e Temperos" onclick="tc(this)">Ervas</span>
            </div>
          </div>
          <div class="fgroup">
            <span class="flabel">☀️ Luz</span>
            <div class="chips-grid">
              <span class="chip" data-f="luz" data-v="sombra" onclick="tc(this)">Sombra</span>
              <span class="chip" data-f="luz" data-v="meia-luz" onclick="tc(this)">Meia-luz</span>
              <span class="chip" data-f="luz" data-v="sol pleno" onclick="tc(this)">Sol pleno</span>
            </div>
          </div>
          <div class="fgroup">
            <span class="flabel">💧 Água</span>
            <div class="chips-grid">
              <span class="chip" data-f="umid" data-v="baixa" onclick="tc(this)">Baixa</span>
              <span class="chip" data-f="umid" data-v="media" onclick="tc(this)">Média</span>
              <span class="chip" data-f="umid" data-v="alta" onclick="tc(this)">Alta</span>
            </div>
          </div>
          <div class="fgroup">
            <span class="flabel">🏖️ Tipo de Solo</span>
            <div class="chips-grid">
              <span class="chip" data-f="terra" data-v="drenada" onclick="tc(this)">Drenada</span>
              <span class="chip" data-f="terra" data-v="organica" onclick="tc(this)">Orgânica</span>
              <span class="chip" data-f="terra" data-v="argilosa" onclick="tc(this)">Argilosa</span>
              <span class="chip" data-f="terra" data-v="substrato" onclick="tc(this)">Substrato</span>
            </div>
          </div>
          <div class="fgroup">
            <span class="flabel">⭐ Nível de Cuidado</span>
            <div class="chips-grid">
              <span class="chip" data-f="diff" data-v="Facil" onclick="tc(this)">Fácil</span>
              <span class="chip" data-f="diff" data-v="Moderado" onclick="tc(this)">Moderado</span>
              <span class="chip" data-f="diff" data-v="Avancado" onclick="tc(this)">Avançado</span>
            </div>
          </div>
        </div>
        <div class="fm-footer">
          <button class="btn-clear" onclick="clf()">Limpar tudo</button>
          <button class="btn-apply" onclick="toggleFilterModal({{force:true}})">Mostrar <span id="fmCount">0</span> plantas</button>
        </div>
      </div>
    </div>
</div>

<div class="overlay" id="ov" onclick="cm(event)">
  <div class="modal" id="mbox">
    <div class="mimg" id="mimg">
      <button class="mclose" onclick="cm({{force:true}})">✕</button>
      <img id="mphoto" src="" alt="" style="display:none">
      <span id="mem"></span>
    </div>
    <div class="mbody">
      <div class="mcat" id="mcat"></div>
      <div class="mname" id="mname"></div>
      <div class="msub" id="msub"></div>
      <div class="mpr">
        <span class="mpr-c" id="mprice"></span>
        <span class="mpr-o" id="morig"></span>
        <span class="mpr-d" id="mdisc"></span>
      </div>
      <div class="mattrs" id="mattrs"></div>
      <div class="mdesc" id="mdesc"></div>
      <button class="mbtn" id="mAddBtn">🛒 Ver produto completo</button>
    </div>
  </div>
</div>

<script src="api.js"></script>
<script>
var PLANTS={plantas_js}, ACT={{cat:[],luz:[],umid:[],terra:[],diff:[]}};

var CAT_EM={{'Plantas de Interior':'🪴','Flores':'🌸','Arvores':'🌳','Árvores':'🌳',
  'Palmeiras':'🌴','Suculentas e Cactos':'🌵','Gramineas e Bambus':'🎋',
  'Gramíneas e Bambus':'🎋','Trepadeiras':'🌿','Samambaias e Fetos':'🌾',
  'Ervas e Temperos':'🌿','Aquaticas':'🌊','Kokedamas':'🪴'}};

var TERRA={{
  drenada:  {{i:'🏖️',l:'Drenada',  c:'tp-dren'}},
  organica: {{i:'🌱',l:'Organica', c:'tp-org'}},
  argilosa: {{i:'🟫',l:'Argilosa', c:'tp-arg'}},
  substrato:{{i:'🪴',l:'Substrato',c:'tp-sub'}}
}};

function norm(s){{return(s||'').normalize('NFD').replace(/[̀-ͯ]/g,'').toLowerCase()}}

function getDiff(p){{
  var diff = p.dificuldade; // Já calculado no Python
  if(diff === 'Avancado') return {{l:'Avancado',c:'d-avanc',i:'🔴'}};
  if(diff === 'Moderado') return {{l:'Moderado',c:'d-mod',  i:'🟡'}};
  return                         {{l:'Facil',   c:'d-facil', i:'🟢'}};
}}

function getTip(p){{
  var u=norm(p.umid_key), luz=p.luz||[];
  if(u==='baixa')  return '💡 Rega espaçada — deixe o solo secar entre regas.';
  if(u==='alta')   return '💧 Mantenha o solo sempre umido. Evite ressecamento.';
  if(luz.length>=3)return '🌿 Versatil! Adapta-se a qualquer ambiente.';
  if(luz.includes('sombra')&&!luz.includes('sol pleno')) return '🌑 Ideal para interiores sem luz direta.';
  if(luz.includes('sol pleno')&&!luz.includes('sombra')) return '☀️ Precisa de sol direto para florescer.';
  return '🌤️ Prefere luz indireta ou meia-sombra.';
}}

function disc(a,o){{if(!o||!a||o<=a)return null;return Math.round((1-a/o)*100)}}

function fmtR(n){{return n?(n.toFixed(2).replace('.',',')):'—'}}

function drops(u){{return{{baixa:1,media:2,media_acc:2,alta:3}}[norm(u)]||1}}

function buildLuzDots(luz){{
  var s=luz.includes('sombra'),m=luz.includes('meia-luz'),so=luz.includes('sol pleno');
  return '<div class="luz-dots">'
    +'<div class="ldot '+(s?'l-sombra':'l-off')+'" title="Sombra">🌑</div>'
    +'<div class="ldot '+(m?'l-meia':'l-off')+'" title="Meia-luz">🌤️</div>'
    +'<div class="ldot '+(so?'l-sol':'l-off')+'" title="Sol pleno">☀️</div>'
    +'</div>'
    +'<span class="luz-text">'+(luz.length>=3?'Qualquer':luz.map(function(x){{return x==='sol pleno'?'Sol':x==='meia-luz'?'Meia':'Sombra'}}).join(', '))+'</span>';
}}

function buildDrops(u,big){{
  var d=drops(u), sz=big?'1.15rem':'.9rem';
  var r='<div class="drops">';
  for(var i=1;i<=3;i++) r+='<span class="drop '+(i<=d?'on':'off')+'" style="font-size:'+sz+'">💧</span>';
  r+='</div><span class="wlabel">'+(u?u.charAt(0).toUpperCase()+u.slice(1):'')+'</span>';
  return r;
}}

function card(p){{
  var diff=getDiff(p), tt=p.terra_key, ti=TERRA[tt];
  var di=disc(p.preco_atual,p.preco_original);
  var em=CAT_EM[p.categoria]||'🌿';
  var luz=p.luz||[];
  var catNorm=norm(p.categoria);

  var luzIcons='';
  if(luz.includes('sombra')) luzIcons+='🌑';
  if(luz.includes('meia-luz')) luzIcons+='🌤️';
  if(luz.includes('sol pleno')) luzIcons+='☀️';
  if(!luzIcons) luzIcons='🌤️';
  
  var dropCount=drops(p.umid_key), dropIcons='';
  for(var i=0;i<dropCount;i++) dropIcons+='💧';

  var el=document.createElement('div');
  el.className='card';
  el.dataset.cat=catNorm;
  el.dataset.luz=JSON.stringify(luz);
  el.dataset.umid=p.umid_key;
  el.dataset.terra=tt;
  el.dataset.diff=diff.l;
  el.dataset.name=norm(p.nome);

  el.innerHTML='<button class="btn-fav" onclick="event.stopPropagation();this.innerHTML=\\\'❤️\\\';this.style.transform=\\\'scale(1.2)\\\';setTimeout(()=>this.style.transform=\\\'\\\',200)">🤍</button>'
    +'<div class="cimg">'
    +'<span class="cimg-em">'+em+'</span>'
    +(p.imagens&&p.imagens[0]?'<img data-src="'+p.imagens[0]+'" alt="'+p.nome+'" onload="this.classList.add(\\\'loaded\\\')" onerror="this.style.display=\\\'none\\\'">':'')
    +(di?'<span class="b-disc">-'+di+'%</span>':'')
    +'</div>'
    +'<div class="cbody">'
    +'<div><div class="cname">'+p.nome+'</div></div>'
    +'<div class="price-row"><span class="price-c">R$ '+fmtR(p.preco_atual)+'</span>'
    +(p.preco_original&&p.preco_original>p.preco_atual?'<span class="price-o">R$ '+fmtR(p.preco_original)+'</span>':'')
    +'</div>'
    +'<div class="card-tech">'
    +'<span title="Luz: '+(luz.join(', ')||'Meia-luz')+'">'+luzIcons+' '+((luz[0]||'Meia').charAt(0).toUpperCase()+(luz[0]||'meia').slice(1))+'</span>'
    +'<span title="Água: '+p.umid_key+'">'+dropIcons+' Rega '+(p.umid_key||'Média')+'</span>'
    +'</div>'
    +'<button class="btn-add-card" onclick="event.stopPropagation();addToCart(this, \\''+p.id+'\\')">+ Adicionar</button>'
    +'</div>';

  el.addEventListener('click',function(){{openM(p)}});
  return el;
}}

function af(){{
  var q=norm(document.getElementById('search').value);
  var cards=document.querySelectorAll('.card');
  var v=0;
  cards.forEach(function(c){{
    var luz=JSON.parse(c.dataset.luz||'[]');
    var ok=(!q||c.dataset.name.includes(q))
      &&(!ACT.cat.length||ACT.cat.some(function(x){{return norm(c.dataset.cat).includes(norm(x))}}))
      &&(!ACT.luz.length||ACT.luz.some(function(l){{return luz.includes(l)}}))
      &&(!ACT.umid.length||ACT.umid.includes(c.dataset.umid))
      &&(!ACT.terra.length||ACT.terra.includes(c.dataset.terra))
      &&(!ACT.diff.length||ACT.diff.includes(c.dataset.diff));
    c.style.display=ok?'':'none';
    if(ok)v++;
  }});
  document.getElementById('vcount').textContent=v;
  document.getElementById('fmCount').textContent=v;
  renderPills();
  var em=document.getElementById('empty-st');
  if(v===0){{
    if(!em){{var e=document.createElement('div');e.id='empty-st';e.className='empty';
      e.innerHTML='<p>🌵</p><p>Nenhuma planta encontrada com esses filtros.</p>';
      document.getElementById('grid').appendChild(e);}}
  }} else if(em) em.remove();
}}

function tc(el){{
  var f=el.dataset.f, v=el.dataset.v, a=ACT[f];
  var i=a.indexOf(v);
  if(i===-1) a.push(v); else a.splice(i,1);
  el.classList.toggle('active',a.includes(v));
  af();
}}

function clf(){{
  Object.keys(ACT).forEach(function(k){{ACT[k]=[]}});
  document.querySelectorAll('.chip.active').forEach(function(c){{c.classList.remove('active')}});
  document.getElementById('search').value='';
  af();
}}

function toggleFilterModal(e){{
  if(e&&e.force){{}} else if(e&&e.target&&e.target!==document.getElementById('filterModalOverlay'))return;
  document.getElementById('filterModalOverlay').classList.toggle('open');
}}

function renderPills(){{
  var c=document.getElementById('activeFiltersBar'); c.innerHTML='';
  var count=0;
  Object.keys(ACT).forEach(function(k){{
    ACT[k].forEach(function(v){{
      count++;
      var p=document.createElement('span');p.className='active-chip';
      p.innerHTML=v+' <button onclick="rp(\\\''+k+'\\\',\\\''+v+'\\\')">✕</button>';
      c.appendChild(p);
    }});
  }});
  c.style.display = count>0 ? 'flex' : 'none';
}}

function rp(f,v){{
  var a=ACT[f],i=a.indexOf(v);if(i!==-1)a.splice(i,1);
  var el=document.querySelector('.chip[data-f="'+f+'"][data-v="'+v+'"]');
  if(el)el.classList.remove('active');
  af();
}}

function addToCart(btn, id){{
  var p=PLANTS.find(x=>x.id===id);
  if(!p)return;
  if(window.Cart) Cart.add(p);
  var orig = btn.innerHTML;
  btn.innerHTML = '✔ Adicionado';
  btn.classList.add('added');
  
  // Animate global cart icon if exists
  var cartBadge = document.querySelector('.cart-count');
  if(cartBadge){{
    cartBadge.style.transform='scale(1.5)';
    setTimeout(()=>{{cartBadge.style.transform='';}}, 300);
  }}

  setTimeout(()=>{{btn.innerHTML=orig;btn.classList.remove('added');}}, 1500);
}}

function openM(p){{
  var diff=getDiff(p),tt=p.terra_key,ti=TERRA[tt];
  var di=disc(p.preco_atual,p.preco_original);
  var luz=p.luz||[];
  document.getElementById('mcat').textContent=p.categoria+(p.subcategoria?' › '+p.subcategoria:'');
  document.getElementById('mname').textContent=p.nome;
  document.getElementById('msub').textContent=diff.i+' Cuidado '+diff.l;
  document.getElementById('mprice').textContent=p.preco_atual?'R$ '+fmtR(p.preco_atual):'';
  document.getElementById('morig').textContent=(p.preco_original&&p.preco_original>p.preco_atual)?'R$ '+fmtR(p.preco_original):'';
  document.getElementById('mdisc').textContent=di?'-'+di+'%':'';
  document.getElementById('mdesc').textContent=p.descricao||'';
  document.getElementById('mem').textContent=CAT_EM[p.categoria]||'🌿';
  var ph=document.getElementById('mphoto');
  if(p.imagens&&p.imagens[0]){{ph.src=p.imagens[0];ph.style.display='';
    document.getElementById('mem').style.display='none';}}
  else{{ph.style.display='none';document.getElementById('mem').style.display='';}}
  document.getElementById('mattrs').innerHTML=
    '<div class="mattr"><span class="mal">Luz</span><div style="display:flex;align-items:center">'+buildLuzDots(luz)+'</div></div>'
    +'<div class="mattr"><span class="mal">Agua</span><div style="display:flex;align-items:center;gap:5px">'+buildDrops(p.umid_key,true)+'</div></div>'
    +'<div class="mattr"><span class="mal">Terra</span><span class="tpill '+ti.c+'" style="font-size:.78rem">'+ti.i+' '+ti.l+'</span><span style="font-size:.72rem;color:#aaa;margin-left:7px">'+( p.tipo_terra||'')+'</span></div>'
    +'<div class="mattr"><span class="mal">Dica</span><span style="font-size:.77rem;color:#33691e;background:#f1f8e9;padding:4px 9px;border-radius:8px">'+getTip(p)+'</span></div>';
  
  // Botão para ver o produto completo (SPA)
  document.getElementById('mAddBtn').onclick = function() {{
      Turbo.visit('produto.html?id=' + p.id);
  }};

  document.getElementById('ov').classList.add('open');
  document.body.style.overflow='hidden';
}}

function cm(e){{
  if(e&&e.force){{}} else if(e&&e.target&&e.target!==document.getElementById('ov'))return;
  document.getElementById('ov').classList.remove('open');
  document.body.style.overflow='';
}}

function lazyLoad(){{
  var obs=new IntersectionObserver(function(entries){{
    entries.forEach(function(e){{
      if(e.isIntersecting){{
        var img=e.target;img.src=img.dataset.src;delete img.dataset.src;obs.unobserve(img);
      }}
    }});
  }},{{rootMargin:'200px'}});
  document.querySelectorAll('img[data-src]').forEach(function(i){{obs.observe(i)}});
}}

function initCatalog(){{
  document.getElementById('hbadge').textContent=PLANTS.length+' plantas disponíveis';
  var g=document.getElementById('grid');
  g.innerHTML = '';
  PLANTS.forEach(function(p){{g.appendChild(card(p))}});
  af();
  lazyLoad();
}}

document.addEventListener('turbo:load', initCatalog);
if (document.readyState !== 'loading') initCatalog();
document.addEventListener('keydown',function(e){{if(e.key==='Escape')cm({{force:true}})}});
</script>
</body>
</html>"""

with open(OUTPUT, "w", encoding="utf-8") as f:
    f.write(HTML)

print(f"[ok] {OUTPUT} gerado com {len(plantas)} plantas!")
