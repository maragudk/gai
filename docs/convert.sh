#!/bin/bash

# Copy the template to a temporary file to use as a base
cp template-quirky-v4.html temp_template.html

# List of pages to convert
PAGES=("getting-started" "concepts" "chat-completion" "embedding" "tools" "evals" "examples")

for page in "${PAGES[@]}"; do
  echo "Converting $page.html to quirky style..."
  
  # Extract the content from the existing page
  # This gets everything between the main content div and the footer div
  content=$(sed -n '/<main class="flex-grow p-6">/,/<footer class="bg-gray-800/p' "$page.html" | sed '1d;$d')
  
  # Create quirky version
  cat template-quirky-v4.html | 
    # Set the correct page title
    sed "s/{{TITLE}}/$(echo $page | sed 's/\([a-z]\)\([a-zA-Z0-9]*\)/\u\1\2/g')/g" |
    # Set the appropriate navigation active state
    sed "s/href=\"$page.html\" class=\"px-3 py-2 hover:bg-yellow-400/href=\"$page.html\" class=\"px-3 py-2 bg-yellow-400 text-black/g" |
    # Insert the page content
    sed "s|{{CONTENT}}|$content|g" |
    # Clear table of contents placeholder
    sed "s/{{TABLE_OF_CONTENTS}}//g" |
    # Clear navigation link placeholders
    sed "s/{{PREV_LINK}}//g" |
    sed "s/{{NEXT_LINK}}//g" > "$page.html.new"
  
  # Replace the old page with the new quirky one
  mv "$page.html.new" "$page.html"
done

# Clean up
rm temp_template.html

echo "Conversion complete!"