-- Everroot botanical reference catalogue.
--
-- Adds the authority layer that lets Everroot offer an exploratory tree path
-- when no reviewed cultural association exists for a selection.
--
-- Provenance discipline:
--   * Every species row records the GBIF Backbone Taxonomy taxon key it was
--     matched to, so each accepted name and every classification field below is
--     reproducible against that dataset.
--   * Only authority-verified data is stored: accepted name, authorship, rank,
--     taxonomic status, family, genus, order, and the protologue citation.
--   * No descriptive prose, habitat, native-range, ecological or cultural claim
--     is stored. Everroot has no reviewed source for those, so it says so
--     instead of filling the gap.
--   * Common names are selected from GBIF's own vernacular-name list for each
--     taxon; entries that are obviously place names or data errors are omitted.
--     Each pick is recorded with the synonym evidence row it came from.
--   * No cultural mapping, review or publication event is created here. The
--     reviewed cultural dataset remains empty.
--
-- The `source_reviews` row below is an automated authority-import check: each
-- accepted name was matched to the GBIF Backbone by exact name. It is not an
-- editorial review of botanical claims, and it is labelled as such.

CREATE TABLE tree_species_authority (
    species_id uuid PRIMARY KEY REFERENCES tree_species(id) ON DELETE RESTRICT,
    authority_dataset text NOT NULL,
    authority_doi text NOT NULL DEFAULT '',
    authority_url text NOT NULL DEFAULT '',
    gbif_taxon_key bigint NOT NULL UNIQUE,
    accepted_name text NOT NULL,
    authorship text NOT NULL DEFAULT '',
    rank text NOT NULL DEFAULT '',
    taxon_status text NOT NULL CHECK (taxon_status IN ('ACCEPTED','SYNONYM','DOUBTFUL')),
    family text NOT NULL DEFAULT '',
    genus text NOT NULL DEFAULT '',
    order_name text NOT NULL DEFAULT '',
    protologue text NOT NULL DEFAULT '',
    evidence_id uuid NOT NULL REFERENCES source_evidence(id) ON DELETE RESTRICT,
    recorded_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX tree_species_authority_family_idx ON tree_species_authority(family, accepted_name);

-- The authority layer is append-only, like every other registry record.
CREATE TRIGGER tree_species_authority_immutable
BEFORE UPDATE OR DELETE ON tree_species_authority
FOR EACH ROW EXECUTE FUNCTION prevent_append_only_mutation();

-- Everroot will not present an authority record for a species that it has not
-- also marked botanically verified.
CREATE FUNCTION ensure_authority_species_verified() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM tree_species WHERE id=NEW.species_id AND botanical_status='verified') THEN
    RAISE EXCEPTION 'authority records require a botanically verified species';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER tree_species_authority_verified
BEFORE INSERT ON tree_species_authority
FOR EACH ROW EXECUTE FUNCTION ensure_authority_species_verified();


-- ---------------------------------------------------------------- authority source
INSERT INTO sources (id, title, source_type, author, publisher, url, bibliographic_reference, language, accessed_on, attribution_requirements, reuse_status)
VALUES (
    '2c90758b-5a3c-58ec-bfac-5302e411fd41',
    'GBIF Backbone Taxonomy',
    'authority_database',
    'GBIF Secretariat',
    'GBIF Secretariat',
    'https://www.gbif.org/dataset/d7dddbf4-2cf0-4f39-9b2a-bb099caae36c',
    'GBIF Secretariat. GBIF Backbone Taxonomy. Checklist dataset. https://doi.org/10.15468/39omei',
    'en',
    DATE '2026-10-17',
    'GBIF Secretariat, GBIF Backbone Taxonomy, https://doi.org/10.15468/39omei (CC BY 4.0)',
    'citation_only'
);

-- Automated authority-import check. Not an editorial review of botanical claims.
INSERT INTO source_reviews (source_id, reviewer_id, reviewer_role, decision, notes)
VALUES (
    '2c90758b-5a3c-58ec-bfac-5302e411fd41',
    'catalogue-import:gbif-authority-match',
    'botanical',
    'approved',
    'Automated check: every accepted name in this import was matched to the GBIF Backbone Taxonomy by exact name and returned ACCEPTED. This records a data-import verification of the name and classification only; it is not a human editorial review of botanical or cultural claims.'
);

UPDATE sources SET verification_status='verified' WHERE id='2c90758b-5a3c-58ec-bfac-5302e411fd41';

INSERT INTO source_permission_material_types (
    source_id, material_type, permission_state, licence_name, licence_url,
    commercial_use_allowed, extraction_allowed, local_storage_allowed,
    redistribution_allowed, adaptation_allowed, translation_allowed,
    attribution_required, attribution_requirements, automated_access_permitted,
    written_permission_required, reviewed_by, review_notes, evidence_urls
) VALUES (
    '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'botanical_data', 'allowed_under_licence', 'CC BY 4.0',
    'https://creativecommons.org/licenses/by/4.0/',
    true, true, true, true, true, true,
    true, 'Attribution to GBIF Secretariat and the GBIF Backbone Taxonomy is required.',
    true, false, 'catalogue-import:gbif-authority-match',
    'The GBIF Backbone Taxonomy is published under CC BY 4.0. Everroot stores name and classification fields only, and cites the taxon key and dataset DOI.',
    ARRAY['https://doi.org/10.15468/39omei']
);


-- ---------------------------------------------------- species and their authority records
INSERT INTO tree_species (id, scientific_name, family, botanical_status) VALUES
    ('b38a17b2-8b49-519a-9e83-4eb30cc97ce2', 'Ficus religiosa', 'Moraceae', 'verified'),
    ('bcb1af64-4ed1-58b7-92e9-803d850c3f70', 'Ficus benghalensis', 'Moraceae', 'verified'),
    ('f31d50a0-a68c-5a07-9f40-e410a467c12a', 'Ficus racemosa', 'Moraceae', 'verified'),
    ('a3063853-c712-5067-a451-d7f962fd77c4', 'Azadirachta indica', 'Meliaceae', 'verified'),
    ('135fc458-af2f-5a19-aa1f-3b6e5f586c07', 'Mangifera indica', 'Anacardiaceae', 'verified'),
    ('b8b77192-978a-5faf-9b09-29ac9d99dcac', 'Saraca asoca', 'Fabaceae', 'verified'),
    ('0145f780-9752-51e9-8ad9-2ca6b0d0a047', 'Butea monosperma', 'Fabaceae', 'verified'),
    ('bc81d71d-a4ce-5ada-9bf2-11978ce9d456', 'Terminalia arjuna', 'Combretaceae', 'verified'),
    ('3253818a-ead9-5bf0-a137-e4fa19f9b163', 'Aegle marmelos', 'Rutaceae', 'verified'),
    ('23d82d31-d4e1-5c90-981e-f64620aabda8', 'Dalbergia sissoo', 'Fabaceae', 'verified'),
    ('41897c38-8913-5677-8d11-3a5aa9770bdc', 'Bombax ceiba', 'Malvaceae', 'verified'),
    ('c0841cd2-9e00-5c14-addc-0aa4db740b25', 'Cassia fistula', 'Fabaceae', 'verified'),
    ('c04fbd95-a546-5356-a942-8e77d73337b5', 'Shorea robusta', 'Dipterocarpaceae', 'verified'),
    ('853221c8-ca36-5872-bae9-17409f0fb40e', 'Santalum album', 'Santalaceae', 'verified'),
    ('fa4db0ce-6ff2-5900-b3d1-9bc3b5c16221', 'Phyllanthus emblica', 'Phyllanthaceae', 'verified'),
    ('516e8bba-e61e-54f2-b434-9d1d0add8beb', 'Prosopis cineraria', 'Fabaceae', 'verified'),
    ('0a2827b8-b2e9-579b-a191-ead2e8854b36', 'Syzygium cumini', 'Myrtaceae', 'verified'),
    ('cc4c6219-da8e-5b93-abb5-2b405992a4dd', 'Madhuca longifolia', 'Sapotaceae', 'verified');

-- One evidence row per species. The locator is the GBIF taxon key, which is what
-- makes each accepted name below independently checkable. No passage text is
-- stored, because the source is registered as citation-only.
INSERT INTO source_evidence (id, source_id, locator, language, evidence_notes) VALUES
    ('5c7a8732-66ef-5126-91c4-7b47ea11a7d8', '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'GBIF Backbone Taxonomy, taxon key 5361935', 'en', 'Accepted-name and classification record matched by exact name. No passage text is stored.'),
    ('403379f9-7081-5c3e-8a50-5bef6866de59', '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'GBIF Backbone Taxonomy, taxon key 5361919', 'en', 'Accepted-name and classification record matched by exact name. No passage text is stored.'),
    ('a764f3b9-ff1c-585f-b693-bf38630eb44a', '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'GBIF Backbone Taxonomy, taxon key 5361904', 'en', 'Accepted-name and classification record matched by exact name. No passage text is stored.'),
    ('328c5907-753c-5afa-9b55-9d33ba628e1c', '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'GBIF Backbone Taxonomy, taxon key 3190474', 'en', 'Accepted-name and classification record matched by exact name. No passage text is stored.'),
    ('f589e82e-38a1-5f9e-a522-29f1b2489e62', '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'GBIF Backbone Taxonomy, taxon key 3190638', 'en', 'Accepted-name and classification record matched by exact name. No passage text is stored.'),
    ('83c4383f-45d8-57c8-81b1-cdbe470373b3', '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'GBIF Backbone Taxonomy, taxon key 7458904', 'en', 'Accepted-name and classification record matched by exact name. No passage text is stored.'),
    ('578a24d4-f201-50e9-bd60-e665ed99856c', '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'GBIF Backbone Taxonomy, taxon key 8234570', 'en', 'Accepted-name and classification record matched by exact name. No passage text is stored.'),
    ('727743da-79f0-5469-91e0-dbceb72fffe1', '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'GBIF Backbone Taxonomy, taxon key 3699548', 'en', 'Accepted-name and classification record matched by exact name. No passage text is stored.'),
    ('44b85dde-6a33-5fbb-b4c2-201005144371', '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'GBIF Backbone Taxonomy, taxon key 5421195', 'en', 'Accepted-name and classification record matched by exact name. No passage text is stored.'),
    ('27a24817-9919-5b44-be1a-3bc624aa2825', '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'GBIF Backbone Taxonomy, taxon key 2968532', 'en', 'Accepted-name and classification record matched by exact name. No passage text is stored.'),
    ('697ab5e2-46f7-5ea1-a7b8-86a651340cc8', '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'GBIF Backbone Taxonomy, taxon key 3152226', 'en', 'Accepted-name and classification record matched by exact name. No passage text is stored.'),
    ('458b5d8b-b173-5805-b946-97ae7332d12c', '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'GBIF Backbone Taxonomy, taxon key 5357108', 'en', 'Accepted-name and classification record matched by exact name. No passage text is stored.'),
    ('770f1e63-1a66-5e10-87e6-b8c1a48c76f4', '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'GBIF Backbone Taxonomy, taxon key 3189684', 'en', 'Accepted-name and classification record matched by exact name. No passage text is stored.'),
    ('4cb146e9-aa9a-58f6-a0f6-d8e8f02671b0', '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'GBIF Backbone Taxonomy, taxon key 2889785', 'en', 'Accepted-name and classification record matched by exact name. No passage text is stored.'),
    ('3c669061-9021-582d-b796-5d6de60cef06', '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'GBIF Backbone Taxonomy, taxon key 5381660', 'en', 'Accepted-name and classification record matched by exact name. No passage text is stored.'),
    ('dc68ed7a-83e2-5610-9466-26a4cdb40d03', '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'GBIF Backbone Taxonomy, taxon key 5358521', 'en', 'Accepted-name and classification record matched by exact name. No passage text is stored.'),
    ('f8ddbc66-5e59-5ad2-9857-9c4b2a6f0216', '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'GBIF Backbone Taxonomy, taxon key 3183840', 'en', 'Accepted-name and classification record matched by exact name. No passage text is stored.'),
    ('9ea45a57-408d-5e9d-8a89-f5814258102a', '2c90758b-5a3c-58ec-bfac-5302e411fd41', 'GBIF Backbone Taxonomy, taxon key 2883346', 'en', 'Accepted-name and classification record matched by exact name. No passage text is stored.');

INSERT INTO tree_species_authority (
    species_id, authority_dataset, authority_doi, authority_url, gbif_taxon_key,
    accepted_name, authorship, rank, taxon_status, family, genus, order_name,
    protologue, evidence_id
) VALUES
    ('b38a17b2-8b49-519a-9e83-4eb30cc97ce2', 'GBIF Backbone Taxonomy', '10.15468/39omei', 'https://www.gbif.org/species/5361935', 5361935, 'Ficus religiosa', 'L.', 'SPECIES', 'ACCEPTED', 'Moraceae', 'Ficus', 'Rosales', 'L. (1753). In: Sp. Pl. 1059.', '5c7a8732-66ef-5126-91c4-7b47ea11a7d8'),
    ('bcb1af64-4ed1-58b7-92e9-803d850c3f70', 'GBIF Backbone Taxonomy', '10.15468/39omei', 'https://www.gbif.org/species/5361919', 5361919, 'Ficus benghalensis', 'L.', 'SPECIES', 'ACCEPTED', 'Moraceae', 'Ficus', 'Rosales', 'L. (1753). In: Sp. Pl. 1059.', '403379f9-7081-5c3e-8a50-5bef6866de59'),
    ('f31d50a0-a68c-5a07-9f40-e410a467c12a', 'GBIF Backbone Taxonomy', '10.15468/39omei', 'https://www.gbif.org/species/5361904', 5361904, 'Ficus racemosa', 'L.', 'SPECIES', 'ACCEPTED', 'Moraceae', 'Ficus', 'Rosales', 'L. (1753). In: Sp. Pl. 1060.', 'a764f3b9-ff1c-585f-b693-bf38630eb44a'),
    ('a3063853-c712-5067-a451-d7f962fd77c4', 'GBIF Backbone Taxonomy', '10.15468/39omei', 'https://www.gbif.org/species/3190474', 3190474, 'Azadirachta indica', 'A.Juss.', 'SPECIES', 'ACCEPTED', 'Meliaceae', 'Azadirachta', 'Sapindales', 'A. Juss. (1830). In: Mém. Mus. Nat. Hist. Paris 19: 220, T. 2, N. 5.', '328c5907-753c-5afa-9b55-9d33ba628e1c'),
    ('135fc458-af2f-5a19-aa1f-3b6e5f586c07', 'GBIF Backbone Taxonomy', '10.15468/39omei', 'https://www.gbif.org/species/3190638', 3190638, 'Mangifera indica', 'L.', 'SPECIES', 'ACCEPTED', 'Anacardiaceae', 'Mangifera', 'Sapindales', 'L. (1753). In: Sp. Pl. 1: 200.', 'f589e82e-38a1-5f9e-a522-29f1b2489e62'),
    ('b8b77192-978a-5faf-9b09-29ac9d99dcac', 'GBIF Backbone Taxonomy', '10.15468/39omei', 'https://www.gbif.org/species/7458904', 7458904, 'Saraca asoca', '(Roxb.) W.J.de Wilde', 'SPECIES', 'ACCEPTED', 'Fabaceae', 'Saraca', 'Fabales', 'Blumea15: 393 (1968)', '83c4383f-45d8-57c8-81b1-cdbe470373b3'),
    ('0145f780-9752-51e9-8ad9-2ca6b0d0a047', 'GBIF Backbone Taxonomy', '10.15468/39omei', 'https://www.gbif.org/species/8234570', 8234570, 'Butea monosperma', '(Lam.) Kuntze', 'SPECIES', 'ACCEPTED', 'Fabaceae', 'Butea', 'Fabales', 'Revis. Gen. Pl.1: 202 (1891)', '578a24d4-f201-50e9-bd60-e665ed99856c'),
    ('bc81d71d-a4ce-5ada-9bf2-11978ce9d456', 'GBIF Backbone Taxonomy', '10.15468/39omei', 'https://www.gbif.org/species/3699548', 3699548, 'Terminalia arjuna', '(Roxb.) Wight & Arn.', 'SPECIES', 'ACCEPTED', 'Combretaceae', 'Terminalia', 'Myrtales', 'Wight, Arn. (1834). In: Prodr. Fl. Ind. Orient. 314, in Textu (Roxb.) Bedd., Fl. Sylv. I. 28.', '727743da-79f0-5469-91e0-dbceb72fffe1'),
    ('3253818a-ead9-5bf0-a137-e4fa19f9b163', 'GBIF Backbone Taxonomy', '10.15468/39omei', 'https://www.gbif.org/species/5421195', 5421195, 'Aegle marmelos', '(L.) Corrêa', 'SPECIES', 'ACCEPTED', 'Rutaceae', 'Aegle', 'Sapindales', 'Corrêa. (1800). In: Trans. Linn. Soc. 5: 223.', '44b85dde-6a33-5fbb-b4c2-201005144371'),
    ('23d82d31-d4e1-5c90-981e-f64620aabda8', 'GBIF Backbone Taxonomy', '10.15468/39omei', 'https://www.gbif.org/species/2968532', 2968532, 'Dalbergia sissoo', 'Roxb. ex DC.', 'SPECIES', 'ACCEPTED', 'Fabaceae', 'Dalbergia', 'Fabales', 'Prodr.2: 416 (1825)', '27a24817-9919-5b44-be1a-3bc624aa2825'),
    ('41897c38-8913-5677-8d11-3a5aa9770bdc', 'GBIF Backbone Taxonomy', '10.15468/39omei', 'https://www.gbif.org/species/3152226', 3152226, 'Bombax ceiba', 'L.', 'SPECIES', 'ACCEPTED', 'Malvaceae', 'Bombax', 'Malvales', 'L. (1753). In: Sp. Pl. 511, Nom. Cons.', '697ab5e2-46f7-5ea1-a7b8-86a651340cc8'),
    ('c0841cd2-9e00-5c14-addc-0aa4db740b25', 'GBIF Backbone Taxonomy', '10.15468/39omei', 'https://www.gbif.org/species/5357108', 5357108, 'Cassia fistula', 'L.', 'SPECIES', 'ACCEPTED', 'Fabaceae', 'Cassia', 'Fabales', 'Sp. Pl.: 377 (1753)', '458b5d8b-b173-5805-b946-97ae7332d12c'),
    ('c04fbd95-a546-5356-a942-8e77d73337b5', 'GBIF Backbone Taxonomy', '10.15468/39omei', 'https://www.gbif.org/species/3189684', 3189684, 'Shorea robusta', 'Gaertn.', 'SPECIES', 'ACCEPTED', 'Dipterocarpaceae', 'Shorea', 'Malvales', 'Gaertn. (1805). In: Fruct. 3: 48, T. 186.', '770f1e63-1a66-5e10-87e6-b8c1a48c76f4'),
    ('853221c8-ca36-5872-bae9-17409f0fb40e', 'GBIF Backbone Taxonomy', '10.15468/39omei', 'https://www.gbif.org/species/2889785', 2889785, 'Santalum album', 'L.', 'SPECIES', 'ACCEPTED', 'Santalaceae', 'Santalum', 'Santalales', 'L. (1753). In: Sp. Pl. 1: 349.', '4cb146e9-aa9a-58f6-a0f6-d8e8f02671b0'),
    ('fa4db0ce-6ff2-5900-b3d1-9bc3b5c16221', 'GBIF Backbone Taxonomy', '10.15468/39omei', 'https://www.gbif.org/species/5381660', 5381660, 'Phyllanthus emblica', 'L.', 'SPECIES', 'ACCEPTED', 'Phyllanthaceae', 'Phyllanthus', 'Malpighiales', 'Sp. Pl.: 982 (1753)', '3c669061-9021-582d-b796-5d6de60cef06'),
    ('516e8bba-e61e-54f2-b434-9d1d0add8beb', 'GBIF Backbone Taxonomy', '10.15468/39omei', 'https://www.gbif.org/species/5358521', 5358521, 'Prosopis cineraria', '(L.) Druce', 'SPECIES', 'ACCEPTED', 'Fabaceae', 'Prosopis', 'Fabales', 'Rep. Bot. Exch. Club Soc. Brit. Isles3: 422 (1913 publ. 1914)', 'dc68ed7a-83e2-5610-9466-26a4cdb40d03'),
    ('0a2827b8-b2e9-579b-a191-ead2e8854b36', 'GBIF Backbone Taxonomy', '10.15468/39omei', 'https://www.gbif.org/species/3183840', 3183840, 'Syzygium cumini', '(L.) Skeels', 'SPECIES', 'ACCEPTED', 'Myrtaceae', 'Syzygium', 'Myrtales', 'Skeels. (1912). In: U. S. Dept. Agr., Bur. Pl. Industr., Bull. 248: 25.', 'f8ddbc66-5e59-5ad2-9857-9c4b2a6f0216'),
    ('cc4c6219-da8e-5b93-abb5-2b405992a4dd', 'GBIF Backbone Taxonomy', '10.15468/39omei', 'https://www.gbif.org/species/2883346', 2883346, 'Madhuca longifolia', '(J.Koenig ex L.) J.F.Macbr.', 'SPECIES', 'ACCEPTED', 'Sapotaceae', 'Madhuca', 'Ericales', 'Contr. Gray Herb., n.s., 53: 17 (1918)', '9ea45a57-408d-5e9d-8a89-f5814258102a');

-- Common names, each tied to the GBIF vernacular-name evidence row for its
-- species. Language values are ISO 639-3 codes as supplied by GBIF.
INSERT INTO tree_names (species_id, name, language, name_type, evidence_id) VALUES
    ('b38a17b2-8b49-519a-9e83-4eb30cc97ce2', 'Peepul Tree', 'eng', 'common', '5c7a8732-66ef-5126-91c4-7b47ea11a7d8'),
    ('b38a17b2-8b49-519a-9e83-4eb30cc97ce2', 'Bodhi tree', 'eng', 'common', '5c7a8732-66ef-5126-91c4-7b47ea11a7d8'),
    ('b38a17b2-8b49-519a-9e83-4eb30cc97ce2', 'sacred fig', 'eng', 'common', '5c7a8732-66ef-5126-91c4-7b47ea11a7d8'),
    ('b38a17b2-8b49-519a-9e83-4eb30cc97ce2', 'pipal', 'hin', 'common', '5c7a8732-66ef-5126-91c4-7b47ea11a7d8'),
    ('bcb1af64-4ed1-58b7-92e9-803d850c3f70', 'Banyan', 'eng', 'common', '403379f9-7081-5c3e-8a50-5bef6866de59'),
    ('bcb1af64-4ed1-58b7-92e9-803d850c3f70', 'Bengal fig', 'eng', 'common', '403379f9-7081-5c3e-8a50-5bef6866de59'),
    ('bcb1af64-4ed1-58b7-92e9-803d850c3f70', 'East Indian figtree', 'eng', 'common', '403379f9-7081-5c3e-8a50-5bef6866de59'),
    ('bcb1af64-4ed1-58b7-92e9-803d850c3f70', 'Bargad', 'hin', 'common', '403379f9-7081-5c3e-8a50-5bef6866de59'),
    ('f31d50a0-a68c-5a07-9f40-e410a467c12a', 'cluster fig', 'eng', 'common', 'a764f3b9-ff1c-585f-b693-bf38630eb44a'),
    ('f31d50a0-a68c-5a07-9f40-e410a467c12a', 'country fig', 'eng', 'common', 'a764f3b9-ff1c-585f-b693-bf38630eb44a'),
    ('f31d50a0-a68c-5a07-9f40-e410a467c12a', 'redwood fig', 'eng', 'common', 'a764f3b9-ff1c-585f-b693-bf38630eb44a'),
    ('a3063853-c712-5067-a451-d7f962fd77c4', 'Neem', 'eng', 'common', '328c5907-753c-5afa-9b55-9d33ba628e1c'),
    ('a3063853-c712-5067-a451-d7f962fd77c4', 'Neem Tree', 'eng', 'common', '328c5907-753c-5afa-9b55-9d33ba628e1c'),
    ('a3063853-c712-5067-a451-d7f962fd77c4', 'Indian lilac', 'eng', 'common', '328c5907-753c-5afa-9b55-9d33ba628e1c'),
    ('135fc458-af2f-5a19-aa1f-3b6e5f586c07', 'Mango', 'eng', 'common', 'f589e82e-38a1-5f9e-a522-29f1b2489e62'),
    ('135fc458-af2f-5a19-aa1f-3b6e5f586c07', 'Mango Tree', 'eng', 'common', 'f589e82e-38a1-5f9e-a522-29f1b2489e62'),
    ('135fc458-af2f-5a19-aa1f-3b6e5f586c07', 'Indian mango', 'eng', 'common', 'f589e82e-38a1-5f9e-a522-29f1b2489e62'),
    ('135fc458-af2f-5a19-aa1f-3b6e5f586c07', 'common mango', 'eng', 'common', 'f589e82e-38a1-5f9e-a522-29f1b2489e62'),
    ('b8b77192-978a-5faf-9b09-29ac9d99dcac', 'Ashoka', 'eng', 'common', '83c4383f-45d8-57c8-81b1-cdbe470373b3'),
    ('b8b77192-978a-5faf-9b09-29ac9d99dcac', 'Ashoka tree', 'eng', 'common', '83c4383f-45d8-57c8-81b1-cdbe470373b3'),
    ('b8b77192-978a-5faf-9b09-29ac9d99dcac', 'Sita Ashok', 'hin', 'common', '83c4383f-45d8-57c8-81b1-cdbe470373b3'),
    ('0145f780-9752-51e9-8ad9-2ca6b0d0a047', 'Flame of The Forest', 'eng', 'common', '578a24d4-f201-50e9-bd60-e665ed99856c'),
    ('0145f780-9752-51e9-8ad9-2ca6b0d0a047', 'पलाश', 'hin', 'common', '578a24d4-f201-50e9-bd60-e665ed99856c'),
    ('0145f780-9752-51e9-8ad9-2ca6b0d0a047', 'পলাশ', 'ben', 'common', '578a24d4-f201-50e9-bd60-e665ed99856c'),
    ('bc81d71d-a4ce-5ada-9bf2-11978ce9d456', 'Arjun tree', 'eng', 'common', '727743da-79f0-5469-91e0-dbceb72fffe1'),
    ('bc81d71d-a4ce-5ada-9bf2-11978ce9d456', 'arjun', 'eng', 'common', '727743da-79f0-5469-91e0-dbceb72fffe1'),
    ('bc81d71d-a4ce-5ada-9bf2-11978ce9d456', 'white murdh', 'eng', 'common', '727743da-79f0-5469-91e0-dbceb72fffe1'),
    ('3253818a-ead9-5bf0-a137-e4fa19f9b163', 'Indian Bael', 'eng', 'common', '44b85dde-6a33-5fbb-b4c2-201005144371'),
    ('3253818a-ead9-5bf0-a137-e4fa19f9b163', 'Indian Quince', 'eng', 'common', '44b85dde-6a33-5fbb-b4c2-201005144371'),
    ('23d82d31-d4e1-5c90-981e-f64620aabda8', 'Indian Rosewood', 'eng', 'common', '27a24817-9919-5b44-be1a-3bc624aa2825'),
    ('23d82d31-d4e1-5c90-981e-f64620aabda8', 'North Indian rosewood', 'eng', 'common', '27a24817-9919-5b44-be1a-3bc624aa2825'),
    ('23d82d31-d4e1-5c90-981e-f64620aabda8', 'Shisham', 'hin', 'common', '27a24817-9919-5b44-be1a-3bc624aa2825'),
    ('41897c38-8913-5677-8d11-3a5aa9770bdc', 'Red Silk Cottontree', 'eng', 'common', '697ab5e2-46f7-5ea1-a7b8-86a651340cc8'),
    ('41897c38-8913-5677-8d11-3a5aa9770bdc', 'silk cottontree', 'eng', 'common', '697ab5e2-46f7-5ea1-a7b8-86a651340cc8'),
    ('41897c38-8913-5677-8d11-3a5aa9770bdc', 'Indian kapok', 'eng', 'common', '697ab5e2-46f7-5ea1-a7b8-86a651340cc8'),
    ('c0841cd2-9e00-5c14-addc-0aa4db740b25', 'Golden Shower Tree', 'eng', 'common', '458b5d8b-b173-5805-b946-97ae7332d12c'),
    ('c0841cd2-9e00-5c14-addc-0aa4db740b25', 'Indian Laburnum', 'eng', 'common', '458b5d8b-b173-5805-b946-97ae7332d12c'),
    ('c0841cd2-9e00-5c14-addc-0aa4db740b25', 'Golden Shower', 'eng', 'common', '458b5d8b-b173-5805-b946-97ae7332d12c'),
    ('c04fbd95-a546-5356-a942-8e77d73337b5', 'sal tree', 'eng', 'common', '770f1e63-1a66-5e10-87e6-b8c1a48c76f4'),
    ('c04fbd95-a546-5356-a942-8e77d73337b5', 'shal', 'hin', 'common', '770f1e63-1a66-5e10-87e6-b8c1a48c76f4'),
    ('c04fbd95-a546-5356-a942-8e77d73337b5', 'Sal', 'ben', 'common', '770f1e63-1a66-5e10-87e6-b8c1a48c76f4'),
    ('853221c8-ca36-5872-bae9-17409f0fb40e', 'Indian Sandalwood', 'eng', 'common', '4cb146e9-aa9a-58f6-a0f6-d8e8f02671b0'),
    ('853221c8-ca36-5872-bae9-17409f0fb40e', 'white sandalwood', 'eng', 'common', '4cb146e9-aa9a-58f6-a0f6-d8e8f02671b0'),
    ('853221c8-ca36-5872-bae9-17409f0fb40e', 'chandan', 'hin', 'common', '4cb146e9-aa9a-58f6-a0f6-d8e8f02671b0'),
    ('fa4db0ce-6ff2-5900-b3d1-9bc3b5c16221', 'Indian Gooseberry', 'eng', 'common', '3c669061-9021-582d-b796-5d6de60cef06'),
    ('fa4db0ce-6ff2-5900-b3d1-9bc3b5c16221', 'Emblic', 'eng', 'common', '3c669061-9021-582d-b796-5d6de60cef06'),
    ('fa4db0ce-6ff2-5900-b3d1-9bc3b5c16221', 'emblic myrobalan', 'eng', 'common', '3c669061-9021-582d-b796-5d6de60cef06'),
    ('516e8bba-e61e-54f2-b434-9d1d0add8beb', 'Indian mesquite', 'eng', 'common', 'dc68ed7a-83e2-5610-9466-26a4cdb40d03'),
    ('516e8bba-e61e-54f2-b434-9d1d0add8beb', 'khejri', 'hin', 'common', 'dc68ed7a-83e2-5610-9466-26a4cdb40d03'),
    ('516e8bba-e61e-54f2-b434-9d1d0add8beb', 'jand', 'hin', 'common', 'dc68ed7a-83e2-5610-9466-26a4cdb40d03'),
    ('0a2827b8-b2e9-579b-a191-ead2e8854b36', 'Jamun', 'eng', 'common', 'f8ddbc66-5e59-5ad2-9857-9c4b2a6f0216'),
    ('0a2827b8-b2e9-579b-a191-ead2e8854b36', 'Java plum', 'eng', 'common', 'f8ddbc66-5e59-5ad2-9857-9c4b2a6f0216'),
    ('0a2827b8-b2e9-579b-a191-ead2e8854b36', 'Black Plum', 'eng', 'common', 'f8ddbc66-5e59-5ad2-9857-9c4b2a6f0216'),
    ('0a2827b8-b2e9-579b-a191-ead2e8854b36', 'Malabar plum', 'eng', 'common', 'f8ddbc66-5e59-5ad2-9857-9c4b2a6f0216'),
    ('cc4c6219-da8e-5b93-abb5-2b405992a4dd', 'mahua', 'eng', 'common', '9ea45a57-408d-5e9d-8a89-f5814258102a'),
    ('cc4c6219-da8e-5b93-abb5-2b405992a4dd', 'moatree', 'eng', 'common', '9ea45a57-408d-5e9d-8a89-f5814258102a'),
    ('cc4c6219-da8e-5b93-abb5-2b405992a4dd', 'mowra-buttertree', 'eng', 'common', '9ea45a57-408d-5e9d-8a89-f5814258102a');
