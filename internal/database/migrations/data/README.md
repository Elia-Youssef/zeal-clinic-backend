# Reference Data Sources

This directory contains static reference datasets embedded into database seeds.

## Datasets

### lebanon_cities.json
- **Content:** Lebanese localities with their governorate and district.
- **Source:** Administrative datasets for Lebanon of the United Nations Office for the Coordination of Humanitarian Affairs (OCHA), published on the Humanitarian Data Exchange (HDX): https://data.humdata.org/
- **License / Terms:** Creative Commons Attribution 3.0 IGO (CC BY-IGO 3.0): https://creativecommons.org/licenses/by/3.0/igo/
- **Changes:** Only the locality, governorate and district names are kept, and each row carries an identifier generated for this application. OCHA does not endorse this application.

### countries.json
- **Content:** Country names only (factual data).
- **Source:** A widely shared public list of country names.
- **License / Terms:** The upstream list states no license.
- **Changes:** Obvious spelling and encoding errors of the source list are corrected (migrations `00014` and `00015` repair databases seeded before the corrections), and each row carries an identifier generated for this application.
