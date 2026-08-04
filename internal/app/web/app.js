fetch('/api/v1/health/ready')
  .then(response => response.ok ? response.json() : Promise.reject(new Error('not ready')))
  .then(() => { document.querySelector('#status').textContent = 'Player service ready'; })
  .catch(() => { document.querySelector('#status').textContent = 'Player service unavailable'; });
