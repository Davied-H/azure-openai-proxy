import os
os.environ['NO_PROXY'] = 'localhost,127.0.0.1'

from openai import OpenAI

if __name__ == '__main__':
    client = OpenAI(
        base_url="http://localhost:8080/v1",
        api_key="sk-your-api-key"
    )

    response = client.embeddings.create(
        model="text-embedding-ada-002",
        input="Hello, world!"
    )

    print(f"Embedding 维度: {len(response.data[0].embedding)}")
    print(f"Token 使用: {response.usage}")
