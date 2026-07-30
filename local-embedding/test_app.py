import requests

def test_health():
    r = requests.get('http://localhost:4000/healthz')
    assert r.status_code == 200

def test_embeddings():
    r = requests.post('http://localhost:4000/v1/embeddings', json={"input": ["测试"]})
    assert r.status_code == 200
    data = r.json()
    assert data['model'].startswith('iic/')
    assert len(data['data'][0]['embedding']) == 512
