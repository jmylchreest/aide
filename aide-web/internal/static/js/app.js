// aide-web application JavaScript
document.addEventListener('DOMContentLoaded', function() {
  if (typeof htmx !== 'undefined') {
    htmx.config.defaultSwapStyle = 'innerHTML';
  }

  // Sortable tables: click column headers to sort
  document.querySelectorAll('table.sortable thead th').forEach(function(th) {
    th.style.cursor = 'pointer';
    th.addEventListener('click', function() {
      var table = th.closest('table');
      var tbody = table.querySelector('tbody');
      var rows = Array.from(tbody.querySelectorAll('tr'));
      var idx = Array.from(th.parentNode.children).indexOf(th);
      var asc = th.dataset.sort !== 'asc';

      // Clear sort indicators
      th.parentNode.querySelectorAll('th').forEach(function(h) {
        h.dataset.sort = '';
        h.classList.remove('sort-asc', 'sort-desc');
      });
      th.dataset.sort = asc ? 'asc' : 'desc';
      th.classList.add(asc ? 'sort-asc' : 'sort-desc');

      rows.sort(function(a, b) {
        var at = (a.children[idx] || {}).textContent || '';
        var bt = (b.children[idx] || {}).textContent || '';
        // Try numeric sort first
        var an = parseFloat(at), bn = parseFloat(bt);
        if (!isNaN(an) && !isNaN(bn)) return asc ? an - bn : bn - an;
        return asc ? at.localeCompare(bt) : bt.localeCompare(at);
      });
      rows.forEach(function(r) { tbody.appendChild(r); });
    });
  });

  // Client-side table filter: type in search box to filter rows
  document.querySelectorAll('[data-filter-table]').forEach(function(input) {
    var tableId = input.dataset.filterTable;
    input.addEventListener('input', function() {
      var q = input.value.toLowerCase();
      var table = document.getElementById(tableId);
      if (!table) return;
      table.querySelectorAll('tbody tr').forEach(function(row) {
        row.style.display = row.textContent.toLowerCase().includes(q) ? '' : 'none';
      });
    });
  });
});

// Re-init after HTMX swaps
if (typeof htmx !== 'undefined') {
  htmx.on('htmx:afterSwap', function() {
    // Re-attach sortable handlers after HTMX content swap
    document.querySelectorAll('table.sortable thead th').forEach(function(th) {
      if (th.dataset.sortBound) return;
      th.dataset.sortBound = '1';
      th.style.cursor = 'pointer';
      th.addEventListener('click', function() {
        var table = th.closest('table');
        var tbody = table.querySelector('tbody');
        var rows = Array.from(tbody.querySelectorAll('tr'));
        var idx = Array.from(th.parentNode.children).indexOf(th);
        var asc = th.dataset.sort !== 'asc';
        th.parentNode.querySelectorAll('th').forEach(function(h) {
          h.dataset.sort = '';
          h.classList.remove('sort-asc', 'sort-desc');
        });
        th.dataset.sort = asc ? 'asc' : 'desc';
        th.classList.add(asc ? 'sort-asc' : 'sort-desc');
        rows.sort(function(a, b) {
          var at = (a.children[idx] || {}).textContent || '';
          var bt = (b.children[idx] || {}).textContent || '';
          var an = parseFloat(at), bn = parseFloat(bt);
          if (!isNaN(an) && !isNaN(bn)) return asc ? an - bn : bn - an;
          return asc ? at.localeCompare(bt) : bt.localeCompare(at);
        });
        rows.forEach(function(r) { tbody.appendChild(r); });
      });
    });
  });
}
