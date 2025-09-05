from pathlib import Path
import os
import requests
import json
import yaml
import sys
from random import choice
from urllib.parse import urlencode

# Configuration for different environments
ENVIRONMENTS = {
    "local": {
        "yaml_file": Path.home() / "idcarioca_staging.yaml",  # Use staging config for local
        "api_base_url": "http://localhost:8080",
        "requires_auth": True
    },
    "staging": {
        "yaml_file": Path.home() / "idcarioca_staging.yaml",
        "api_base_url": "https://services.staging.app.dados.rio/rmi",
        "requires_auth": True
    },
    "prod": {
        "yaml_file": Path.home() / "idcarioca_prod.yaml",
        "api_base_url": "https://services.pref.rio/rmi",
        "requires_auth": True
    }
}

class APIClient:
    """
    API client that handles base URL, authentication, and common HTTP operations.
    """
    
    def __init__(self, environment="staging"):
        self.environment = environment.lower()
        if self.environment not in ENVIRONMENTS:
            raise ValueError(f"Invalid environment: {environment}. Use 'local', 'staging', or 'prod'")
        
        self.config = ENVIRONMENTS[self.environment]
        self.base_url = self.config["api_base_url"]
        self.access_token = get_access_token(environment) if self.config["requires_auth"] else None
        
        # Set up default headers
        self.headers = {
            'Content-Type': 'application/json',
        }
        
        # Add authorization header if we have a token
        if self.access_token:
            self.headers['Authorization'] = f'Bearer {self.access_token}'
    
    def get(self, path, params=None, headers=None):
        """
        Make a GET request to the API.
        
        Args:
            path (str): API endpoint path (e.g., '/v1/health')
            params (dict): Query parameters
            headers (dict): Additional headers to include
        
        Returns:
            requests.Response: The HTTP response
        """
        url = f"{self.base_url}{path}"
        request_headers = {**self.headers, **(headers or {})}
        
        print(f"🔍 GET {url}")
        if params:
            print(f"   Query params: {params}")
        
        response = requests.get(url, params=params, headers=request_headers, timeout=30)
        
        print(f"   Status: {response.status_code}")
        return response
    
    def post(self, path, data=None, json_data=None, headers=None):
        """
        Make a POST request to the API.
        
        Args:
            path (str): API endpoint path (e.g., '/v1/citizen/12345678901/address')
            data (dict): Form data to send
            json_data (dict): JSON data to send
            headers (dict): Additional headers to include
        
        Returns:
            requests.Response: The HTTP response
        """
        url = f"{self.base_url}{path}"
        request_headers = {**self.headers, **(headers or {})}
        
        print(f"📤 POST {url}")
        if json_data:
            print(f"   JSON data: {json.dumps(json_data, indent=2)}")
        elif data:
            print(f"   Form data: {data}")
        
        response = requests.post(url, data=data, json=json_data, headers=request_headers, timeout=30)
        
        print(f"   Status: {response.status_code}")
        return response
    
    def put(self, path, data=None, json_data=None, headers=None):
        """
        Make a PUT request to the API.
        
        Args:
            path (str): API endpoint path (e.g., '/v1/citizen/12345678901/address')
            data (dict): Form data to send
            json_data (dict): JSON data to send
            headers (dict): Additional headers to include
        
        Returns:
            requests.Response: The HTTP response
        """
        url = f"{self.base_url}{path}"
        request_headers = {**self.headers, **(headers or {})}
        
        print(f"🔄 PUT {url}")
        if json_data:
            print(f"   JSON data: {json.dumps(json_data, indent=2)}")
        elif data:
            print(f"   Form data: {data}")
        
        response = requests.put(url, data=data, json=json_data, headers=request_headers, timeout=30)
        
        print(f"   Status: {response.status_code}")
        return response
    
    def delete(self, path, headers=None):
        """
        Make a DELETE request to the API.
        
        Args:
            path (str): API endpoint path
            headers (dict): Additional headers to include
        
        Returns:
            requests.Response: The HTTP response
        """
        url = f"{self.base_url}{path}"
        request_headers = {**self.headers, **(headers or {})}
        
        print(f"🗑️ DELETE {url}")
        
        response = requests.delete(url, headers=request_headers, timeout=30)
        
        print(f"   Status: {response.status_code}")
        return response

def load_config(environment="staging"):
    """
    Load OAuth configuration from YAML or JSON files.
    """
    if environment.lower() not in ENVIRONMENTS:
        raise ValueError(f"Invalid environment: {environment}. Use 'local', 'staging', or 'prod'")
    
    config = ENVIRONMENTS[environment.lower()]
    config_file = config["yaml_file"]
    
    if not config_file.exists():
        raise FileNotFoundError(f"Configuration file not found: {config_file}")
    
    print(f"📁 Loading configuration from: {config_file}")
    
    try:
        with open(config_file, 'r') as f:
            # Try YAML first, then JSON as fallback
            try:
                file_config = yaml.safe_load(f)
            except yaml.YAMLError:
                # If YAML fails, try JSON
                f.seek(0)  # Reset file pointer
                file_config = json.load(f)
        
        # Extract OAuth2 configuration
        if 'oauth2' in file_config:
            oauth_config = file_config['oauth2']
        else:
            # If no oauth2 section, assume the config is directly OAuth2
            oauth_config = file_config
        
        # Validate required fields
        required_fields = ['issuer', 'client_id', 'client_secret']
        missing_fields = [field for field in required_fields if field not in oauth_config]
        
        if missing_fields:
            raise ValueError(f"Missing required OAuth2 fields: {missing_fields}")
        
        print(f"✅ Configuration loaded successfully for {environment} environment")
        return oauth_config
        
    except (yaml.YAMLError, json.JSONDecodeError) as e:
        raise ValueError(f"Failed to parse configuration file: {e}")
    except Exception as e:
        raise Exception(f"Error loading configuration: {e}")

def get_access_token(environment="staging"):
    """
    Generate an access token using OAuth2 client credentials flow.
    Similar to the implementation in k6/load_test.js
    """
    try:
        # Check if environment requires authentication
        if environment.lower() not in ENVIRONMENTS:
            raise ValueError(f"Invalid environment: {environment}")
        
        if not ENVIRONMENTS[environment.lower()]["requires_auth"]:
            print("🏠 Local mode - no access token required")
            return None
        
        # Load OAuth configuration from file
        oauth_config = load_config(environment)
        
        issuer = oauth_config['issuer']
        client_id = oauth_config['client_id']
        client_secret = oauth_config['client_secret']
        
        print("🔐 Fetching Keycloak JWT token...")
        print(f"   Issuer: {issuer}")
        print(f"   Client ID: {client_id}")
        
        # Prepare token request
        token_url = f"{issuer}/protocol/openid-connect/token"
        token_data = {
            'client_id': client_id,
            'client_secret': client_secret,
            'grant_type': 'client_credentials',
            'scope': 'profile email'
        }
        
        headers = {
            'Content-Type': 'application/x-www-form-urlencoded'
        }
        
        # Make the token request
        response = requests.post(
            token_url,
            data=urlencode(token_data),
            headers=headers,
            timeout=30
        )
        
        if response.status_code != 200:
            print(f"❌ Failed to obtain JWT token: {response.status_code}")
            print(f"Response: {response.text}")
            raise Exception(f"Authentication failed with status: {response.status_code}")
        
        # Parse the response
        tokens = response.json()
        
        if 'access_token' not in tokens:
            print("❌ No access token in response")
            print(f"Response: {response.text}")
            raise Exception("No access token received")
        
        print("✅ JWT token obtained successfully")
        print(f"🔑 Token type: {tokens.get('token_type', 'Bearer')}")
        print(f"⏰ Token expires in: {tokens.get('expires_in', 'unknown')} seconds")
        
        return tokens['access_token']
        
    except requests.exceptions.RequestException as e:
        print(f"❌ Request error: {e}")
        raise
    except json.JSONDecodeError as e:
        print(f"❌ Error parsing token response: {e}")
        raise Exception("Invalid token response format")
    except Exception as e:
        print(f"❌ Unexpected error: {e}")
        raise

def get_api_base_url(environment="staging"):
    """
    Get the API base URL for the specified environment.
    """
    if environment.lower() not in ENVIRONMENTS:
        raise ValueError(f"Invalid environment: {environment}. Use 'local', 'staging', or 'prod'")
    
    return ENVIRONMENTS[environment.lower()]["api_base_url"]

def main():
    """Main function to demonstrate token generation and API client usage"""
    # Parse command line arguments for environment
    environment = "staging"  # default
    if len(sys.argv) > 1:
        env_arg = sys.argv[1].lower()
        if env_arg in ["local", "staging", "prod"]:
            environment = env_arg
        else:
            print(f"❌ Invalid environment: {env_arg}")
            print("   Usage: python main.py [local|staging|prod]")
            print("   Default: staging")
            return 1
    
    print(f"🚀 Starting setup for {environment} environment...")
    
    try:
        # Get API base URL
        api_base_url = get_api_base_url(environment)
        print(f"🌐 API Base URL: {api_base_url}")
        
        # Get access token (if needed)
        access_token = get_access_token(environment)
        
        if access_token:
            print(f"\n🎉 Successfully generated access token!")
            print(f"Token preview: {access_token[:50]}...")
        else:
            print(f"\n🏠 Local mode - ready for unauthenticated requests")
        
        print(f"\n📋 Environment Summary:")
        print(f"   Environment: {environment}")
        print(f"   API Base URL: {api_base_url}")
        print(f"   Authentication: {'Required' if access_token else 'Not Required'}")
        
        # Create API client for easy usage
        print(f"\n🔧 Creating API client...")
        api_client = APIClient(environment)
        
        print(f"\n✨ API client ready!")
        # Example: Test health endpoint
        print(f"\n🧪 Testing health endpoint...")
        try:
            health_response = api_client.get('/v1/health')
            if health_response.status_code == 200:
                health_data = health_response.json()
                print(f"✅ Health check successful: {health_data.get('status', 'unknown')}")
            else:
                print(f"⚠️ Health check returned status: {health_response.status_code}")
        except Exception as e:
            print(f"⚠️ Health check failed: {e}")

        # # Test CF lookup functionality with CPFs that exist in different environments
        # print("Testing CF lookup with CPFs from staging vs local...")
        
        # if environment == "staging":
        #     # Use staging CPFs
        #     cpf_with_cf = "47562396507"  # Has CF data in staging
        #     cpf_without_cf = "45049725810"  # Should get CF via MCP lookup
        # else:
        #     # Use CPFs that actually exist in local mock database
        #     cpf_with_cf = "47562396507"  # Has CF data (indicador: true)
        #     cpf_without_cf = "45049725810"  # Should get CF via MCP lookup (indicador: false)
        
        # test_cases = [
        #     {
        #         'name': 'Copacabana (Known CF Address)',
        #         'address': {
        #             'logradouro': 'Avenida Atlântica',
        #             'numero': '1702',
        #             'complemento': None,
        #             'bairro': 'Copacabana',
        #             'municipio': 'Rio de Janeiro',
        #             'estado': 'RJ',
        #             'cep': '22021001'
        #         },
        #         'expect_cf': True  # This address has been confirmed to have a CF
        #     },
        #     {
        #         'name': 'Ipanema (Tourist Area)',
        #         'address': {
        #             'logradouro': 'Rua Visconde de Pirajá',
        #             'numero': '500',
        #             'complemento': None,
        #             'bairro': 'Ipanema',
        #             'municipio': 'Rio de Janeiro',
        #             'estado': 'RJ',
        #             'cep': '22410002'
        #         },
        #         'expect_cf': None  # Unknown if CF exists
        #     },
        #     {
        #         'name': 'Cidade Nova (Original Test Address)',
        #         'address': {
        #             'logradouro': 'Rua Afonso Cavalcanti',
        #             'numero': '455',
        #             'complemento': None,
        #             'bairro': 'Cidade Nova',
        #             'municipio': 'Rio de Janeiro',
        #             'estado': 'RJ',
        #             'cep': '20211110'
        #         },
        #         'expect_cf': False  # Previously confirmed no CF
        #     }
        # ]

        # address = choice(test_cases)
        # print(f"Testing address: {address['name']}")
        # response = api_client.put(f"/v1/citizen/{cpf_without_cf}/address", json_data=address['address'])
        # print(response.json())
        # response.raise_for_status()
        
        # print(f"\nTesting CPF without CF data: {cpf_without_cf}")
        # response2 = api_client.get(f"/v1/citizen/{cpf_without_cf}/wallet")
        # print(response2.json())

        r = api_client.get('/v1/citizen/45049725810')
        data = r.json()
        print(data)
        print(f"Exhibition Name: {data.get('nome_exibicao')}")

        r = api_client.put('/v1/citizen/45049725810/exhibition-name', json_data={'valor': 'John Doe'})
        print(r.json())

        r = api_client.get('/v1/citizen/45049725810')
        data = r.json()
        print(data)
        print(f"Exhibition Name: {data.get('nome_exibicao')}")

    except Exception as e:
        print(f"\n💥 Failed to setup environment: {e}")
        return 1
    
    return 0

if __name__ == "__main__":
    exit(main())