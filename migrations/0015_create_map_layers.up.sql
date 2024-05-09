CREATE TABLE map_layers
(
    id                 BIGSERIAL PRIMARY KEY,
    name               TEXT,
    capabilities_url   TEXT,
    layer              TEXT,
    matrix_set         TEXT,
    resolutions        FLOAT[],
    default_resolution FLOAT,
    os_branding        BOOLEAN,
    extra_attributions TEXT[]
);

CREATE TABLE region_map_layers
(
    region_id    BIGINT REFERENCES regions (id),
    map_layer_id BIGINT REFERENCES map_layers (id),
    PRIMARY KEY (region_id, map_layer_id)
);

INSERT INTO map_layers (name, capabilities_url, layer, matrix_set, resolutions, default_resolution, os_branding)
VALUES ('OS Leisure',
        'https://api.os.uk/maps/raster/v1/wmts?request=getcapabilities&service=WMTS&key=zr5c2eAF5GgLTTbOqOBQ4sphVZSGYCy6',
        'Leisure_27700', 'EPSG:27700',
        '{13.999999997795275, 6.999999998897637, 3.4999999994488187, 1.7499999997244093}', 6.999999998897637, TRUE);

INSERT INTO map_layers (name, capabilities_url, layer, matrix_set, resolutions, default_resolution, extra_attributions)
VALUES ('USGS Topo',
        'https://basemap.nationalmap.gov/arcgis/rest/services/USGSTopo/MapServer/WMTS/1.0.0/WMTSCapabilities.xml',
        'USGSTopo', 'default028mm',
        '{38.21851414253181, 19.109257071265905, 9.554628535632952, 4.777314267948769, 2.3886571339743843}',
        9.554628535632952,
        '{"USGS The National Map: National Boundaries Dataset, 3DEP Elevation Program, Geographic Names Information System, National Hydrography Dataset, National Land Cover Database, National Structures Dataset, and National Transportation Dataset; USGS Global Ecosystems; U.S. Census Bureau TIGER/Line data; USFS Road data; Natural Earth Data; U.S. Department of State HIU; NOAA National Centers for Environmental Information"}');

INSERT INTO map_layers (name, capabilities_url, layer, matrix_set, resolutions, default_resolution, extra_attributions)
VALUES ('FS Topo',
        'https://apps.fs.usda.gov/arcx/rest/services/EDW/EDW_FSTopo_01/MapServer/WMTS/1.0.0/WMTSCapabilities.xml',
        'EDW_EDW_FSTopo_01', 'default028mm',
        '{19.109257071265894, 9.554628535632947, 4.777314267948769, 2.3886571339743794, 1.1943285668548997}',
        9.554628535632947, '{"USFS, Esri, TomTom, FAO, NOAA, USGS"}');
