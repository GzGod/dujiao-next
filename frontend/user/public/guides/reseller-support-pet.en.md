# Storefront support pet guide

The floating pet opens your storefront's own Telegram support link. It changes the appearance of the support entry, not orders or payments.

1. Browse https://petdex.dev/ and open a pet page. The main store uses Jiyi: https://petdex.dev/pets/jiyi .
2. Open **Share / More actions → Download ZIP**. You do not need the install command or desktop software.
3. Extract the ZIP. Find `spritesheet.webp` or `spritesheet.png`. Upload that image, not the ZIP or `pet.json`.
4. Sign in on the main store. Open **Reseller console → Store settings → Support** and configure your own Telegram contact.
5. Under Support pet, select **Standard Petdex sheet**, then upload the extracted spritesheet image.
6. Optionally add the pet name and its original Petdex page URL. Check the preview; hover to preview waving.
7. Click **Save configuration** at the bottom. Refresh your storefront and verify both the pet and support destination.

## Supported formats

Spritesheets use **8 columns × 9 rows**. Jiyi is 1536 × 1872, with 192 × 208 frames. The first row supplies idle frames, and the fourth row supplies waving frames. Other layouts are unsupported; choose a compatible pet instead. Do not upload an entire spritesheet in ordinary image mode.

For a logo, single character picture or animated GIF, select **Image / GIF**. PNG, WebP, JPG and GIF are supported. GIF keeps its own animation. The site's upload size and dimension limits still apply.

## Restore and troubleshoot

- Choose Default Jiyi and save to restore the default, or Hide support pet and save to hide the entry.
- A valid Telegram support contact is required. The main store's support contact is never substituted for a reseller.
- If you see a grid of tiny characters, select spritesheet mode. If validation fails, use the original spritesheet from the ZIP, not a screenshot.
- Uploading alone does not publish the change. Always save configuration. If an image is missing, upload it again and save; uploads use the site's persistent storage.
- The optional source must be the original `https://petdex.dev/pets/pet-slug` page, not a download URL.

Petdex hosts user-submitted artwork. Check the creator's licensing and attribution terms for your storefront; a source link is not a commercial-use licence.
