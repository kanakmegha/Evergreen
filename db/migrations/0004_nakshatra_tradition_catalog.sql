-- Catalog scaffolding only; no tree mapping or cultural claim is inserted.
INSERT INTO cultures (slug, display_name) VALUES ('hindu', 'Hindu traditions');
INSERT INTO traditions (culture_id, slug, display_name)
SELECT id, 'nakshatra-tree-association', 'Nakshatra tree association' FROM cultures WHERE slug='hindu';
INSERT INTO tradition_locales (tradition_id, locale, region_code, region_name, variant_label)
SELECT id, 'ta-IN', 'IN-TN', 'Tamil Nadu', 'Tamil reference context' FROM traditions WHERE slug='nakshatra-tree-association';
INSERT INTO tradition_locales (tradition_id, locale, region_code, region_name, variant_label)
SELECT id, 'hi-IN', 'IN-NORTH', 'North India', 'North Indian reference context' FROM traditions WHERE slug='nakshatra-tree-association';
