"""
Enriquece produtos.json com classificação botânica — sem API, 100% offline.

Adiciona: categoria, subcategoria, luz, umidade, tipo_terra, preço original corrigido.
Remove:   marca, estoque, sku.
"""

import csv
import json
import math
import random
from collections import Counter

INPUT_FILE = "data/produtos.json"
OUTPUT_JSON = "data/produtos_enriquecidos.json"
OUTPUT_CSV = "data/produtos_enriquecidos.csv"

# ─────────────────────────────────────────────────────────────────────────────
# Base de conhecimento botânico — (palavras-chave no nome) → atributos
# ─────────────────────────────────────────────────────────────────────────────
REGRAS = [
    # ── GRAMÍNEAS E BAMBUS ────────────────────────────────────────────────
    {
        "keys": ["grama ", "grama-", "gramado"],
        "categoria": "Gramíneas e Bambus", "subcategoria": "Gramados",
        "luz": ["sol pleno"], "umidade": "média", "tipo_terra": "fértil e bem drenada",
    },
    {
        "keys": ["bambu"],
        "categoria": "Gramíneas e Bambus", "subcategoria": "Bambus",
        "luz": ["meia-luz", "sol pleno"], "umidade": "alta", "tipo_terra": "úmida e rica em matéria orgânica",
    },
    # ── PALMEIRAS ─────────────────────────────────────────────────────────
    {
        "keys": ["areca", "palmeira", "licuala", "fênix", "jerivá", "tamareira", "coco"],
        "categoria": "Palmeiras", "subcategoria": "Palmeiras Ornamentais",
        "luz": ["meia-luz", "sol pleno"], "umidade": "média", "tipo_terra": "arenosa e bem drenada",
    },
    # ── SUCULENTAS E CACTOS ────────────────────────────────────────────────
    {
        "keys": ["cacto", "cactus", "suculenta"],
        "categoria": "Suculentas e Cactos", "subcategoria": "Cactos",
        "luz": ["sol pleno"], "umidade": "baixa", "tipo_terra": "arenosa e bem drenada",
    },
    {
        "keys": ["agave", "furcraea"],
        "categoria": "Suculentas e Cactos", "subcategoria": "Agaves",
        "luz": ["sol pleno"], "umidade": "baixa", "tipo_terra": "arenosa e bem drenada",
    },
    {
        "keys": ["aloe", "babosa"],
        "categoria": "Suculentas e Cactos", "subcategoria": "Suculentas",
        "luz": ["meia-luz", "sol pleno"], "umidade": "baixa", "tipo_terra": "arenosa e bem drenada",
    },
    {
        "keys": ["zamioculca", "zamio"],
        "categoria": "Suculentas e Cactos", "subcategoria": "Suculentas",
        "luz": ["sombra", "meia-luz"], "umidade": "baixa", "tipo_terra": "bem drenada",
    },
    # ── SAMAMBAIAS E FETOS ─────────────────────────────────────────────────
    {
        "keys": ["samambaia", "avenca", "feto ", "asplenium"],
        "categoria": "Samambaias e Fetos", "subcategoria": "Samambaias",
        "luz": ["sombra", "meia-luz"], "umidade": "alta", "tipo_terra": "rica em matéria orgânica e úmida",
    },
    {
        "keys": ["xaxim"],
        "categoria": "Samambaias e Fetos", "subcategoria": "Xaxim",
        "luz": ["meia-luz"], "umidade": "alta", "tipo_terra": "substrato orgânico úmido",
    },
    # ── ÁRVORES FRUTÍFERAS ─────────────────────────────────────────────────
    {
        "keys": ["acerola", "limão", "laranja", "abacate", "goiaba", "manga", "mamão",
                 "uva ", "maracujá", "pitanga", "jabuticaba", "caqui", "pêssego",
                 "ameixa", "figo ", "nespera", "carambola", "romã"],
        "categoria": "Árvores", "subcategoria": "Frutíferas",
        "luz": ["sol pleno"], "umidade": "média", "tipo_terra": "fértil, argilosa e bem irrigada",
    },
    {
        "keys": ["uva vitória", "uva "],
        "categoria": "Árvores", "subcategoria": "Frutíferas",
        "luz": ["sol pleno"], "umidade": "média", "tipo_terra": "fértil e bem drenada",
    },
    # ── ÁRVORES ORNAMENTAIS ────────────────────────────────────────────────
    {
        "keys": ["ipê", "sibipiruna", "flamboiã", "flamboyant", "tipuana", "jacarandá",
                 "paineira", "quaresmeira", "manacá", "resedá", "extremosa"],
        "categoria": "Árvores", "subcategoria": "Ornamentais",
        "luz": ["sol pleno"], "umidade": "média", "tipo_terra": "fértil e bem drenada",
    },
    {
        "keys": ["árvore", "arvore"],
        "categoria": "Árvores", "subcategoria": "Ornamentais",
        "luz": ["sol pleno"], "umidade": "média", "tipo_terra": "fértil e bem drenada",
    },
    # ── TREPADEIRAS ───────────────────────────────────────────────────────
    {
        "keys": ["bouganville", "bougainville", "alamanda", "passiflora", "maracujá",
                 "amor agarradinho", "trepadeira", "cipó", "hera ", "jasmim-",
                 "tumbérgia", "tumburgia", "trialis"],
        "categoria": "Trepadeiras", "subcategoria": "Trepadeiras Ornamentais",
        "luz": ["meia-luz", "sol pleno"], "umidade": "média", "tipo_terra": "fértil e bem drenada",
    },
    # ── ORQUÍDEAS ─────────────────────────────────────────────────────────
    {
        "keys": ["orquídea", "orquidea", "cattleya", "dendrobium", "phalaenopsis",
                 "oncidium", "vanda"],
        "categoria": "Flores", "subcategoria": "Orquídeas",
        "luz": ["meia-luz"], "umidade": "média", "tipo_terra": "substrato para orquídeas (casca de pinus)",
    },
    # ── BROMÉLIAS ─────────────────────────────────────────────────────────
    {
        "keys": ["bromélia", "bromelia", "vriesea", "tillandsia", "neoregelia",
                 "guzmania"],
        "categoria": "Flores", "subcategoria": "Bromélias",
        "luz": ["meia-luz", "sol pleno"], "umidade": "média", "tipo_terra": "substrato drenante para bromélias",
    },
    # ── FLORES DE JARDIM ──────────────────────────────────────────────────
    {
        "keys": ["boca de leão", "azaleia", "begônia", "begonia", "calêndula",
                 "crisântemo", "rosa ", "gérbera", "gerbera", "copo-de-leite",
                 "antúrio", "anturio", "lírio", "lirio", "tulipa", "dália",
                 "dalia", "peônia", "gazânia", "petúnia", "petunia", "violeta",
                 "amor-perfeito", "lavanda", "hortênsia", "hortensia"],
        "categoria": "Flores", "subcategoria": "Flores de Jardim",
        "luz": ["meia-luz", "sol pleno"], "umidade": "média", "tipo_terra": "rica em matéria orgânica",
    },
    {
        "keys": ["azulzinha", "bela-emília", "bela emilia", "baby sun", "rosinha do sol",
                 "acalifa", "rabo de gato"],
        "categoria": "Flores", "subcategoria": "Flores de Jardim",
        "luz": ["meia-luz", "sol pleno"], "umidade": "média", "tipo_terra": "bem drenada e fértil",
    },
    {
        "keys": ["alpínia", "alpinia", "helicônia", "heliconia", "strelitzia",
                 "ave-do-paraíso", "ave do paraiso"],
        "categoria": "Flores", "subcategoria": "Flores Tropicais",
        "luz": ["meia-luz", "sol pleno"], "umidade": "alta", "tipo_terra": "úmida e rica em matéria orgânica",
    },
    # ── ERVAS E TEMPEROS ──────────────────────────────────────────────────
    {
        "keys": ["arruda", "hortelã", "hortelã", "manjericão", "manjericao",
                 "alecrim", "tomilho", "sálvia", "salvia", "camomila", "erva-",
                 "melissa", "capim-limão", "capim limao", "boldo"],
        "categoria": "Ervas e Temperos", "subcategoria": "Plantas Medicinais e Aromáticas",
        "luz": ["meia-luz", "sol pleno"], "umidade": "média", "tipo_terra": "bem drenada",
    },
    {
        "keys": ["alface", "cebolinha", "couve", "rúcula", "rucula", "salsa ",
                 "manjericão", "tomate", "pimenta"],
        "categoria": "Ervas e Temperos", "subcategoria": "Horta",
        "luz": ["sol pleno"], "umidade": "alta", "tipo_terra": "fértil e úmida",
    },
    # ── PLANTAS DE INTERIOR — FOLHAGEM ────────────────────────────────────
    {
        "keys": ["aglaonema", "ficus lyrata", "ficus elastica", "ficus benjamin",
                 "dracena", "dracaena", "yucca", "yuca", "potos", "pothos",
                 "filodendro", "philodendron", "monstera", "costela-de-adão",
                 "costela de adão", "maranta", "calathea", "dieffenbachia",
                 "comigo ninguém pode", "comigo ninguem pode", "sansevieria",
                 "espada de são jorge", "espada de sao jorge", "língua-de-sogra",
                 "língua de sogra", "pilea", "peperômia", "peperomia",
                 "spathiphyllum", "lírio da paz"],
        "categoria": "Plantas de Interior", "subcategoria": "Folhagem Tropical",
        "luz": ["sombra", "meia-luz"], "umidade": "média", "tipo_terra": "bem drenada e rica em matéria orgânica",
    },
    {
        "keys": ["alocasia", "colocasia", "xanthosoma"],
        "categoria": "Plantas de Interior", "subcategoria": "Folhagem Tropical",
        "luz": ["meia-luz"], "umidade": "alta", "tipo_terra": "úmida e rica em matéria orgânica",
    },
    {
        "keys": ["árvore da felicidade", "arvore da felicidade", "pachira",
                 "schefflera", "cheflera", "cica", "cícade"],
        "categoria": "Plantas de Interior", "subcategoria": "Plantas de Grande Porte",
        "luz": ["meia-luz", "sol pleno"], "umidade": "média", "tipo_terra": "bem drenada e fértil",
    },
    {
        "keys": ["aspargo", "asparagus"],
        "categoria": "Plantas de Interior", "subcategoria": "Folhagem",
        "luz": ["meia-luz"], "umidade": "média", "tipo_terra": "bem drenada",
    },
    {
        "keys": ["croton", "cróton", "cronton"],
        "categoria": "Plantas de Interior", "subcategoria": "Folhagem Colorida",
        "luz": ["meia-luz", "sol pleno"], "umidade": "média", "tipo_terra": "fértil e bem drenada",
    },
    {
        "keys": ["zebrina", "trapoeraba"],
        "categoria": "Plantas de Interior", "subcategoria": "Pendentes e Rasteiras",
        "luz": ["sombra", "meia-luz", "sol pleno"], "umidade": "média", "tipo_terra": "bem drenada",
    },
    {
        "keys": ["xanadu"],
        "categoria": "Plantas de Interior", "subcategoria": "Folhagem Tropical",
        "luz": ["meia-luz"], "umidade": "média", "tipo_terra": "bem drenada e rica em matéria orgânica",
    },
    # ── KOKEDAMAS ─────────────────────────────────────────────────────────
    {
        "keys": ["kokedama"],
        "categoria": "Kokedamas", "subcategoria": "Kokedamas",
        "luz": ["sombra", "meia-luz"], "umidade": "alta", "tipo_terra": "substrato para kokedama (musgo e argila)",
    },
    # ── AQUÁTICAS / JARDIM ÚMIDO ──────────────────────────────────────────
    {
        "keys": ["aguapé", "papiro", "papyrus", "taboa", "lótus", "lotus",
                 "nenúfar", "nenufar"],
        "categoria": "Aquáticas", "subcategoria": "Plantas Aquáticas",
        "luz": ["sol pleno"], "umidade": "alta", "tipo_terra": "substrato aquático ou solo encharcado",
    },
    # ── BAMBU DA SORTE / SORTUDAS ─────────────────────────────────────────
    {
        "keys": ["bambu da sorte"],
        "categoria": "Plantas de Interior", "subcategoria": "Plantas da Sorte",
        "luz": ["sombra", "meia-luz"], "umidade": "alta", "tipo_terra": "substrato leve ou apenas água",
    },
    # ── JASMIM ───────────────────────────────────────────────────────────
    {
        "keys": ["jasmim", "jasmin"],
        "categoria": "Flores", "subcategoria": "Flores Perfumadas",
        "luz": ["meia-luz", "sol pleno"], "umidade": "média", "tipo_terra": "fértil e bem drenada",
    },
    # ── PLANTAS MEDICINAIS GENÉRICAS ──────────────────────────────────────
    {
        "keys": ["babosa"],
        "categoria": "Suculentas e Cactos", "subcategoria": "Suculentas Medicinais",
        "luz": ["meia-luz", "sol pleno"], "umidade": "baixa", "tipo_terra": "arenosa e bem drenada",
    },
]

# Padrão genérico (fallback)
FALLBACK = {
    "categoria": "Plantas de Interior",
    "subcategoria": "Ornamentais",
    "luz": ["meia-luz", "sol pleno"],
    "umidade": "média",
    "tipo_terra": "fértil e bem drenada",
}


def classificar(nome: str) -> dict:
    """Retorna os atributos botânicos com base no nome da planta."""
    nome_lower = nome.lower()
    for regra in REGRAS:
        for chave in regra["keys"]:
            if chave in nome_lower:
                return {
                    "categoria": regra["categoria"],
                    "subcategoria": regra["subcategoria"],
                    "luz": list(regra["luz"]),
                    "umidade": regra["umidade"],
                    "tipo_terra": regra["tipo_terra"],
                }
    return dict(FALLBACK)


def fix_price(preco_atual, preco_original):
    """Garante preço original > atual. Retorna (original, atual) como float."""
    try:
        atual = float(preco_atual) if preco_atual else 0.0
    except (ValueError, TypeError):
        atual = 0.0
    try:
        original = float(preco_original) if preco_original else 0.0
    except (ValueError, TypeError):
        original = 0.0

    if atual > 0 and original <= atual:
        mult = random.uniform(1.20, 1.55)
        raw = atual * mult
        base = int(raw)
        frac = raw - base
        original = float(base + (0.99 if frac >= 0.50 else 0.90))

    return round(original, 2), round(atual, 2)


def main():
    print("=== Enriquecimento de Produtos (offline) ===\n")

    with open(INPUT_FILE, encoding="utf-8") as f:
        produtos = json.load(f)

    print(f"[ok] {len(produtos)} produtos carregados\n")

    enriquecidos = []

    for produto in produtos:
        nome = produto.get("nome", "")
        classif = classificar(nome)
        preco_original, preco_atual = fix_price(
            produto.get("preco_atual"), produto.get("preco_original")
        )

        enriquecido = {
            "nome": nome,
            "preco_atual": preco_atual,
            "preco_original": preco_original,
            "categoria": classif["categoria"],
            "subcategoria": classif["subcategoria"],
            "luz": classif["luz"],
            "umidade": classif["umidade"],
            "tipo_terra": classif["tipo_terra"],
            "descricao": produto.get("descricao", ""),
            "imagens": produto.get("imagens", []),
            "url": produto.get("url", ""),
        }
        enriquecidos.append(enriquecido)
        print(f"  {nome[:55]:<55} -> {classif['categoria']}")

    # ── Salva JSON ──────────────────────────────────────────────────────────
    with open(OUTPUT_JSON, "w", encoding="utf-8") as f:
        json.dump(enriquecidos, f, ensure_ascii=False, indent=2)
    print(f"\n[ok] {OUTPUT_JSON} salvo — {len(enriquecidos)} produtos")

    # ── Salva CSV ───────────────────────────────────────────────────────────
    campos = [
        "nome", "preco_atual", "preco_original",
        "categoria", "subcategoria",
        "luz", "umidade", "tipo_terra",
        "descricao", "imagens", "url",
    ]
    with open(OUTPUT_CSV, "w", newline="", encoding="utf-8-sig") as f:
        writer = csv.DictWriter(f, fieldnames=campos, extrasaction="ignore")
        writer.writeheader()
        for p in enriquecidos:
            row = dict(p)
            row["luz"] = " | ".join(p.get("luz") or [])
            row["imagens"] = " | ".join(p.get("imagens") or [])
            writer.writerow(row)
    print(f"[ok] {OUTPUT_CSV} salvo")

    # ── Estatísticas ────────────────────────────────────────────────────────
    print("\n=== Distribuição por Categoria ===")
    for cat, n in Counter(p["categoria"] for p in enriquecidos).most_common():
        print(f"  {cat:40s} {n:3d} plantas")

    print("\n=== Distribuição por Luz ===")
    luz_count: Counter = Counter()
    for p in enriquecidos:
        for l in p.get("luz") or []:
            luz_count[l] += 1
    for luz, n in luz_count.most_common():
        print(f"  {luz:20s} {n:3d}")

    print("\n=== Distribuição por Umidade ===")
    for u, n in Counter(p["umidade"] for p in enriquecidos).most_common():
        print(f"  {u:10s} {n:3d}")

    print("\nConcluído!")


if __name__ == "__main__":
    main()
