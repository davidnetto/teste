"""
Scraper - plantadelivery.com.br
Percorre todas as páginas de /plantas, coleta links de produtos e
extrai informações de cada produto, salvando em produtos.json e produtos.csv.

Dependências:
    pip install requests beautifulsoup4
"""

import csv
import json
import re
import time
from urllib.parse import urljoin

import requests
from bs4 import BeautifulSoup

BASE_URL = "https://www.plantadelivery.com.br"
LIST_PATH = "/plantas"
OUTPUT_JSON = "data/produtos.json"
OUTPUT_CSV = "data/produtos.csv"
DELAY = 1.0  # segundos entre requests (seja gentil com o servidor)

HEADERS = {
    "User-Agent": (
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
        "AppleWebKit/537.36 (KHTML, like Gecko) "
        "Chrome/124.0.0.0 Safari/537.36"
    ),
    "Accept-Language": "pt-BR,pt;q=0.9",
}

session = requests.Session()
session.headers.update(HEADERS)


# ---------------------------------------------------------------------------
# Listagem
# ---------------------------------------------------------------------------

def get_product_links_from_page(page: int) -> list[str]:
    """Retorna todos os hrefs de produto de uma página de listagem."""
    url = f"{BASE_URL}{LIST_PATH}?pagina={page}"
    resp = session.get(url, timeout=30)
    resp.raise_for_status()
    soup = BeautifulSoup(resp.text, "html.parser")

    links = set()
    # Produtos ficam em <li> com link direto para slug do produto
    for a in soup.select("ul li a[href]"):
        href = a["href"]
        # Filtra links que são produtos (não são páginas de categoria, nav, etc.)
        full = urljoin(BASE_URL, href)
        if full.startswith(BASE_URL) and _is_product_link(full, soup):
            links.add(full)

    return list(links)


def _is_product_link(url: str, page_soup: BeautifulSoup) -> bool:
    """Heurística: produto não contém '?' e não é uma seção conhecida do site."""
    path = url.replace(BASE_URL, "").strip("/")
    excluded = {
        "", "plantas", "carrinho", "login", "minha-conta", "busca",
        "frete-gratis", "contato", "sobre", "politica", "trocas",
    }
    # Exclui paths com sub-pastas (categorias) e páginas conhecidas
    if "/" in path:
        return False
    if path in excluded:
        return False
    if path.startswith("?") or path.startswith("#"):
        return False
    # Confirma que o link aparece dentro de um bloco de produto
    a_tags = page_soup.find_all("a", href=lambda h: h and path in h)
    for a in a_tags:
        parent = a.find_parent(["li", "div"])
        if parent and parent.find("img"):  # produtos têm imagem
            return True
    return False


def get_last_page() -> int:
    """Detecta o número da última página na paginação."""
    url = f"{BASE_URL}{LIST_PATH}?pagina=1"
    resp = session.get(url, timeout=30)
    resp.raise_for_status()
    soup = BeautifulSoup(resp.text, "html.parser")

    max_page = 1
    for a in soup.find_all("a", href=True):
        m = re.search(r"pagina=(\d+)", a["href"])
        if m:
            max_page = max(max_page, int(m.group(1)))

    # Verifica também texto de links numéricos que podem não ter href com pagina=
    for a in soup.find_all("a"):
        txt = a.get_text(strip=True)
        if txt.isdigit():
            max_page = max(max_page, int(txt))

    print(f"[paginação] última página detectada: {max_page}")
    return max_page


# ---------------------------------------------------------------------------
# Produto
# ---------------------------------------------------------------------------

def scrape_product(url: str) -> dict:
    """Extrai informações de uma página de produto."""
    resp = session.get(url, timeout=30)
    resp.raise_for_status()
    soup = BeautifulSoup(resp.text, "html.parser")

    data = {"url": url}

    # Nome
    h1 = soup.find("h1")
    data["nome"] = h1.get_text(strip=True) if h1 else ""

    # Preços (original e atual)
    data["preco_original"] = ""
    data["preco_atual"] = ""
    # Padrão comum: preço riscado + preço em destaque
    price_tags = soup.find_all(string=re.compile(r"R\$\s*[\d.,]+"))
    prices = []
    for t in price_tags:
        clean = re.sub(r"[^\d,]", "", t.strip()).replace(",", ".")
        if clean:
            try:
                prices.append(float(clean))
            except ValueError:
                pass
    if len(prices) >= 2:
        data["preco_original"] = prices[0]
        data["preco_atual"] = prices[1]
    elif len(prices) == 1:
        data["preco_atual"] = prices[0]

    # SKU / Código
    sku_match = re.search(r"[Cc][oó]digo[:\s]+([A-Z0-9\-]+)", soup.get_text())
    data["sku"] = sku_match.group(1).strip() if sku_match else ""

    # Estoque
    estoque_match = re.search(
        r"[Ee]stoque[:\s]+([^\n<]+)", soup.get_text()
    )
    data["estoque"] = estoque_match.group(1).strip() if estoque_match else ""

    # Marca
    marca_match = re.search(r"[Mm]arca[:\s]+([^\n<]+)", soup.get_text())
    data["marca"] = marca_match.group(1).strip() if marca_match else ""

    # Categorias (breadcrumb)
    breadcrumb = []
    for crumb in soup.select("ol.breadcrumb li, .breadcrumb a, nav[aria-label] a"):
        txt = crumb.get_text(strip=True)
        if txt and txt.lower() not in ("início", "home"):
            breadcrumb.append(txt)
    data["categorias"] = " > ".join(breadcrumb) if breadcrumb else ""

    # Descrição
    desc_el = (
        soup.find("div", class_=re.compile(r"descri", re.I))
        or soup.find("section", class_=re.compile(r"descri", re.I))
        or soup.find(id=re.compile(r"descri", re.I))
    )
    if not desc_el:
        # Fallback: maior bloco de texto da página
        candidates = soup.find_all(["p", "div"], string=re.compile(r".{80,}"))
        if candidates:
            desc_el = max(candidates, key=lambda e: len(e.get_text()))
    data["descricao"] = desc_el.get_text(" ", strip=True)[:2000] if desc_el else ""

    # Imagens (URLs em alta resolução)
    imagens = []
    for img in soup.find_all("img", src=True):
        src = img["src"]
        if "cdn.awsli.com.br" in src and "/produto/" in src:
            # Troca dimensão para 1200x1200 ou maior disponível
            src_hd = re.sub(r"/\d+x\d+/", "/1200x1200/", src)
            if src_hd not in imagens:
                imagens.append(src_hd)
    data["imagens"] = imagens

    return data


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    print("=== Scraper Planta Delivery ===\n")

    # 1. Descobre número de páginas
    last_page = get_last_page()

    # 2. Coleta todos os links de produto
    all_links: set[str] = set()
    for page in range(1, last_page + 1):
        print(f"[listagem] página {page}/{last_page} ...", end=" ")
        try:
            links = get_product_links_from_page(page)
            all_links.update(links)
            print(f"{len(links)} links (total: {len(all_links)})")
        except Exception as e:
            print(f"ERRO: {e}")
        time.sleep(DELAY)

    print(f"\n[scraping] {len(all_links)} produtos encontrados\n")

    # 3. Extrai dados de cada produto
    produtos = []
    for i, url in enumerate(sorted(all_links), 1):
        print(f"[produto {i}/{len(all_links)}] {url}")
        try:
            produto = scrape_product(url)
            produtos.append(produto)
            print(
                f"  -> {produto.get('nome')} | "
                f"R$ {produto.get('preco_atual')} | "
                f"SKU: {produto.get('sku')}"
            )
        except Exception as e:
            print(f"  -> ERRO: {e}")
        time.sleep(DELAY)

    # 4. Salva JSON
    with open(OUTPUT_JSON, "w", encoding="utf-8") as f:
        json.dump(produtos, f, ensure_ascii=False, indent=2)
    print(f"\n[ok] {OUTPUT_JSON} salvo com {len(produtos)} produtos")

    # 5. Salva CSV
    if produtos:
        campos = [
            "nome", "sku", "preco_atual", "preco_original",
            "estoque", "marca", "categorias", "descricao",
            "imagens", "url",
        ]
        with open(OUTPUT_CSV, "w", newline="", encoding="utf-8-sig") as f:
            writer = csv.DictWriter(f, fieldnames=campos, extrasaction="ignore")
            writer.writeheader()
            for p in produtos:
                row = dict(p)
                row["imagens"] = " | ".join(p.get("imagens", []))
                writer.writerow(row)
        print(f"[ok] {OUTPUT_CSV} salvo")

    print("\nConcluído!")


if __name__ == "__main__":
    main()
